package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"fmt"
	"log"
	"net"
	"os"
	"bufio"
	"net/rpc"
	"net/http"
	"os/exec"
	"io/ioutil"
	"sync"

	//"sync"
)

type Piggyback struct {
	//Piggyback message received at Localtime
	//Indicating which node has been detected as failed
	Action string
	Node Nid
	Localtime int64 //using time.NanoUnix() / 1e6 as local time (in millisecond)
}

type Nid struct {
	IP string
	Timestamp int64
}

type Request struct {
	Action string
	//Content []Nid
	Content []Piggyback
	Node []Nid //Only used for JOIN or LEAVE type, storing current node or membership list to send
}

type Args struct {
	S1, S2 string
}

type Reply struct {
	S, COUNT string
}

type result struct {
	res, count, vm string
}


type RPC string
//type Grep string

//types for RPC for SDFS
//type SdfsNode string
//type SdfsMaster string

type MasterServiceArgs struct {
	Myid Nid
	Request string
	UpdateState []UpdateResult
	Sdfsname string
}
type ReplyToNode struct {
	Dstids []Nid
	Version int
	Replicas map[int][]string
}
type NodeServiceArgs struct {
	Myid Nid
	Request string
	Version int
	Sdfsname string
	LocalDir string
	Meta map[string][]ReplicasState
}
type MasterArgs struct {

}
type ReplyToMaster struct {

}
type ReplicasState map[Nid]bool //type for storing replica name and its status(update success or not)
type UpdateResult struct {
	//The return value of a sdfs file update, including putFile() and reReplicate()
	Id Nid
	Version int
	Success bool
}


const (
	PORT = 6666
	INTRODUCER = "172.22.156.115" //vm1 is the introducer
	INTRO_PORT = 6667
	RPC_PORT = ":7002"
	//GREP_PORT = ":1288"

	//consts for SDFS
	USER = "zz40"//change to your user name for VM
	PASSWORD = "Zhzh#96zzl"//change to your password for ssh to remote VM

	LOCAL_DIR_BASE = "/home/zz40/mp3/"//Used when get file
	SDFS_DIR_BASE = "/home/zz40/mp3/sdfs/"//change the SDFS file storage dir in each node
	//delete existing file and mkdir this dir when initialization

	//REPLICA_PORT = 7000
	//MASTER_PORT = 7001
	NUM_REPLICA = 4

)

var mu sync.Mutex
//var mutex = &sync.Mutex{}
var pb []Piggyback //Every time pb was sent, it first need to be checked for existing time
var ackbuf Request //Global variable for storing ACK for ping
var membership []Nid
var protocStart string //Start of protocol period TODO: need to be changed to proper timer type
var myid Nid
var idxmap map[string]int
var ch = make(chan Request)

//var for SDFS
//var masterSequence
//var nodeSequence
var masterId Nid//Master ID stored in each node
var MetaFile = make(map[string][]ReplicasState)//map from sdfsfile to Replicas.
//In this [][]Nid, the ith element []Nid stores replicas where version i file stores
//var updated [][]bool

func main() {
	go server()
	//go swimDetect()
	go rpcServer()
	//go MasterServer()
	go handleInput()

	initialize()
	printList(membership)
	printID(myid)
	swimDetect()
	//test()

}

func handleInput() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Please enter number for commands:")
		fmt.Println("1-printmember; 2-printid; 3-join; 4-leave; or grep and press enter")
		fmt.Print("Enter text:")
		text, _ := reader.ReadString('\n')
		//TODO: not sure where to put server election

		switch text {
		case "1\n":
			printList(membership)
		case "2\n":
			printID(myid)
		case "3\n":
			join()
		case "4\n":
			leave()
		case "5\n":
			vmAddresses, err := ReadLines("vmlist.txt")
			if err != nil {
				log.Fatal("Error read lines:", err)
			}
			grepcommand, _ := reader.ReadString('\n')
			termargs := Args{grepcommand, "vmlog"}
			grepch := make(chan result)
			for i := 0; i < len(vmAddresses); i++ {
				go grepClient(vmAddresses[i], termargs, grepch)
			}
			m := make(map[string]string)
			for i := 0; i < len(vmAddresses); i++ {
				res := <-grepch
				m[res.vm] = res.count
				fmt.Printf("%s\n", res.res)
				fmt.Printf("%s:%s\n", res.vm, res.count)
			}
			for k, v := range m {
				fmt.Println("vm:", k, "count:", v)
			}
		case "put\n":
			//TODO: can be changed to regexp matching for keyboard command input
			fmt.Println("Enter local file name:")
			localName, _ := reader.ReadString('\n')
			localName = strings.TrimRight(localName, "\n")
			fmt.Println("Enter sdfs file name:")
			sdfsName, _ := reader.ReadString('\n')
			sdfsName = strings.TrimRight(sdfsName, "\n")
			putFile(localName, sdfsName)
		case "get\n":
			fmt.Println("Enter sdfs file name:")
			sdfsName, _ := reader.ReadString('\n')
			sdfsName = strings.TrimRight(sdfsName, "\n")
			fmt.Println("Enter local file name:")
			localName, _ := reader.ReadString('\n')
			localName = strings.TrimRight(localName, "\n")
			getFile(sdfsName, localName)
		case "delete\n":
			fmt.Println("Enter sdfs file name:")
			sdfsName, _ := reader.ReadString('\n')
			sdfsName = strings.TrimRight(sdfsName, "\n")
			deleteFile(sdfsName)
		case "ls\n":
			fmt.Println("Enter sdfs file name:")
			sdfsName, _ := reader.ReadString('\n')
			sdfsName = strings.TrimRight(sdfsName, "\n")
			lsFile(sdfsName)
		case "store\n":
			lsStoredFile()
		}
	}
}

func initialize() {
	fmt.Println("-------------Initializing--------------")

	err := os.Remove("vmlog")
	if err != nil {
		log.Println(err)
	}
	newFile, err := os.Create("vmlog")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(newFile)
	newFile.Close()

	err = os.RemoveAll(SDFS_DIR_BASE)
	if err != nil {
		fmt.Println("error remove sdfs path", err)
	}
	err = os.Mkdir(SDFS_DIR_BASE, 0700)//Permission value 0700: read, write and execute
	if err != nil {
		fmt.Println("error mkdir sdfs path", err)
	}

	myid = Nid{getMyIP(), 0}

	nips, err := ReadLines("vmlist.txt")
	idxmap = readMapping(nips)
	if err != nil {
		log.Print("Read vmlist failed.")
	}
	fmt.Println("-------------Initialization finished--------------")
}

func printList(msList []Nid) {
	fmt.Println("-------------Printing membership list--------------")
	for _, vm := range msList {
		fmt.Println("VM:", vm.IP, " Timestamp:", vm.Timestamp)
	}
	fmt.Println("Total number:", len(msList))
}

func printID(id Nid) {
	fmt.Println("My ID:", id.IP, id.Timestamp)
}

func join() {
	request := Request{"JOIN", nil, nil}
	err := sendRequst(request, INTRODUCER, INTRO_PORT)
	if err != nil {
		log.Print("join failed: ", err)
	}
}

func leave() {
	myidx := getIdx(myid)
	if myidx == -1 {
		log.Fatal("myidx is -1")
	}
	size := len(membership)
	for idx := myidx + size - 2; idx <= myidx+size + 3; idx++ {
		thisidx := idx % size
		if thisidx == idx {
			continue
		}
		host := membership[thisidx]
		if host.IP == myid.IP {
			continue
		}
		request := Request{"LEAVE", nil, []Nid{myid} }
		err := sendRequst(request, host.IP, PORT)
		if err != nil {
			log.Print("leave message sent failed, continue leaving: ", err)
		}
	}
	request := Request{"LEAVE", nil, []Nid{myid} }
	err := sendRequst(request, INTRODUCER, INTRO_PORT)
	if err != nil {
		log.Print("leave message sent to introducer failed, continue leaving: ", err)
	}
	membership = nil
}

func swimDetect() {
	for {
		//fmt.Println("Swim protocol restart")
		//mutex.Lock()
		//defer mutex.Unlock()
		size := len(membership)
		if size <= 1 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		myidx := getIdx(myid)
		//fmt.Println("myidx:", myidx)
		if myidx == -1 {
			log.Fatal("myidx is -1 in swim")
		}
	L:
		for idx := myidx + size - 4; idx <= myidx+size+5; idx++ {
			thisidx := ( (idx % size) + size) % size
			//fmt.Println("thisidx:", thisidx)
			//fmt.Println("thisidx:", thisidx, "len:", len(membership))
			if size != len(membership) {
				break
			}
			host := membership[thisidx]
			if host.IP == myid.IP {
				continue
			}
			ackbuf = Request{"", nil, nil}
			ping(host.IP)
			time.Sleep(800 * time.Millisecond) //TODO: maybe change it to time.After format to be more accurate
			//ackbuf = <- ch
			select {
			case ackbuf = <-ch:
				updatePiggyback(ackbuf)
				break
			default:
				fmt.Println("no message received")
				//Delete non-ack node from membership list
				mu.Lock()
				membership = append(membership[:thisidx], membership[thisidx+1:]...)
				mu.Unlock()

				if host.IP != masterId.IP {
					//start a re-replicate
					fmt.Println("Node fail:", host, ", request master for re-replicate.")
					masterServiceArgs := MasterServiceArgs{host, "FAIL", nil, ""}
					go masterRPCCall(masterId.IP, "RPC.MasterService", masterServiceArgs)
				} else {
					//update master id
					fmt.Println("Master fail, update new leader.")
					go updateLeader()
				}

				//TODO: If I am master, I should immediately call data re-replication
				//TODO: If fail node is master, I should immediately call leader election( update myself), then if all leader elected, data-replication
				pb = append(pb, Piggyback{"FAIL", host, time.Now().UnixNano() / 1e6})
				request := Request{"FAIL", nil, []Nid{host} }
				err := sendRequst(request, INTRODUCER, INTRO_PORT)
				if err != nil {
					log.Print("fail message sent to introducer failed, continue leaving: ", err)
				}
				break L
			}
		}
	}
}

func updatePiggyback(ack Request) {
	/*for idx, item := range pb {
		//delete outdated piggybacks
		if time.Now().UnixNano() / 1e6 - item.Localtime > 2e3 {
			pb = append(pb[:idx], pb[idx+1:]...)
		}
	}*/
	for _, message := range ack.Content {
		//TODO: Think abouto this logic, is it right?
		if message.Node.IP == myid.IP {
			continue
		}
		idx := getIdx(message.Node)
		if idx >= 0 {
			//Delete node from membership list
			writeToFile(fmt.Sprintf("fail or leave:%s timestamp:%s\n", message.Node.IP, message.Node.Timestamp))
			mu.Lock()
			membership = append(membership[:idx], membership[idx+1:]...)
			mu.Unlock()
		}
	}
}

func getIdx(id Nid) int {
	for idx, node := range membership {
		if node.IP == id.IP && node.Timestamp == id.Timestamp {
			return idx
		}
	}
	return -1
}

func ping(addr string) {
	//fmt.Println("pinging:", addr)
	request := Request{"PING", nil, nil}
	err := sendRequst(request, addr, PORT)
	if err != nil {
		log.Print("ping failed: ", err)
	}
}

func sendRequst(request Request, addr string, port int) error {
	host := fmt.Sprintf("%s:%d", addr, port)
	conn, err := net.Dial("udp", host)
	if err != nil {
		return err
	}
	defer conn.Close()

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return err
	}

	//Send UDP packets
	conn.Write([]byte(jsonRequest))
	//fmt.Println("Request sent")
	return err
}

func server() {
	fmt.Println("Server start!")
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", PORT))
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	//defer conn.Close()

	for {
		handleRequest(conn)
	}

}

func handleRequest(conn *net.UDPConn) {
	var buf [1024]byte

	n, addr, err := conn.ReadFromUDP(buf[0:])
	if err != nil {
		return
	}

	var request Request
	err = json.Unmarshal(buf[:n], &request)
	if err != nil {
		log.Print(err)
		return
	}

	switch request.Action {
	case "PING":
		//remoteAddr := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
		//fmt.Println("connecting remote address for ping", remoteAddr)

		// Send ACK for ping
		// Update piggyback list before sending
		//mutex.Lock()
		pbtemp := []Piggyback{}
		for _, item := range pb {
			if time.Now().UnixNano() / 1e6 - item.Localtime < 4e3 {
				pbtemp = append(pbtemp, item)
			}
		}
		pb = pbtemp
		response := Request{"ACK", pb, nil}
		//mutex.Unlock()
		jsonResponse, err := json.Marshal(&response)
		if err != nil {
			log.Print(err)
			break
		}
		addr.Port = PORT
		//remoteAddr = fmt.Sprintf("%s:%d", addr.IP, addr.Port)
		//fmt.Println("send back response to:", remoteAddr)
		conn.WriteToUDP(jsonResponse, addr)
	case "ACK":
		// Store ACK content to global piggyback variable
		ch <- request
	case "LEAVE":
		remoteAddr := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
		fmt.Println("someone leave:", remoteAddr)
		leaveidx := getIdx(request.Node[0])
		if leaveidx < 0 {
			fmt.Println("node not found when leave")
			break
		}
		membership = append(membership[:leaveidx], membership[leaveidx+1:]...)
		writeToFile(fmt.Sprintf("leave:%s timestamp:%s\n", request.Node[0].IP, request.Node[0].Timestamp))
		pb = append(pb, Piggyback{"LEAVE", request.Node[0], time.Now().UnixNano() / 1e6})
		printList(membership)
		fmt.Println("---------Delete leave node finished-------")
	case "JOIN":
		//JOIN the node to membership and ACK to introducer
		remoteAddr := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
		fmt.Println("join someone in:", remoteAddr)
		if len(request.Node) != 1 {
			fmt.Println("Error: number of join node returned from introducer != 1")
			break
		}
		writeToFile(fmt.Sprintf("join:%s timestamp:%s\n", request.Node[0].IP, request.Node[0].Timestamp))
		insert(request.Node[0])
		printList(membership)
		fmt.Println("---------Insert new node finished-------")

	case "INIT":
		remoteAddr := fmt.Sprintf("%s:%d", addr.IP, addr.Port)
		fmt.Println("introducer response for init:", remoteAddr)
		membership = request.Node
		for _, n := range membership {
			if n.IP == myid.IP || n.IP == "127.0.0.1" {
				myid.Timestamp = n.Timestamp
				break
			}
		}
		printList(membership)
		fmt.Println("Initialization finished")

	}

}

//Insert a new node to position according to its vm number
func insert(newnode Nid) {
	var idxlist []int
	for idx, node := range membership {
		if node.IP == newnode.IP {
			membership[idx] = newnode
			return
		}
		idxlist = append(idxlist, idxmap[node.IP])
	}
	insert := sort.Search(len(idxlist), func(i int) bool { return idxlist[i] > idxmap[newnode.IP] })
	if len(membership) == 0 {
		membership = []Nid{newnode}
	} else {
		membership = append(membership[:insert], append([]Nid{newnode}, membership[insert:]...)...)
	}
}

func ReadLines(filename string) (out []string, err error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("Open file error:", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("File scanner error:", err)
	}

	return out, err
}

func readMapping(ips []string) map[string]int {
	out := make(map[string]int)
	for i, ip := range ips {
		out[ip] = i
	}
	return out
}

func getMyIP() string {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		fmt.Println(err)
	}

	var currentIP string

	for _, address := range addrs {

		// check the address type and if it is not a loopback the display it
		// = GET LOCAL IP ADDRESS
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				fmt.Println("Current IP address : ", ipnet.IP.String())
				currentIP = ipnet.IP.String()
				return currentIP
			}
		}
	}
	return currentIP
}

func (t *RPC) Grepexec(args *Args, reply *Reply) error {
	S1 := args.S1
	S2 := args.S2

	fmt.Println(S1)
	fmt.Println(S2)
	out, err := exec.Command("grep", S1, S2).Output()

	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	count, err := exec.Command("grep", "-c", S1, S2).Output()

	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	reply.S = string(out)
	reply.COUNT = string(count)
	fmt.Printf("%s",count)
	return nil
}

func rpcServer() {

	rpcConn := new(RPC)
	rpc.Register(rpcConn)
	rpc.HandleHTTP()

	err := http.ListenAndServe(RPC_PORT, nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func grepClient(vm string, termargs Args, resultch chan result) {
	client, err := rpc.DialHTTP("tcp", vm + RPC_PORT)
	if err != nil {
		resultch <- result{ " VM crashed", "0", vm}
		return
	}

	// Synchronous call
	var out Reply
	err = client.Call("RPC.Grepexec", termargs, &out)
	if err != nil {
		resultch <- result{" Grep command illegal or pattern not found in this VM", "0", vm}
		return
	}

	resultch <- result{ out.S, out.COUNT, vm}
	return
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func writeToFile(content string) {
	err := ioutil.WriteFile("vmlog", []byte(content), 0666)
	if err != nil {
		log.Println("writeToFile error: ", err)
	}
}

//---------------------------------SDFS node command functions-----------------------------------------
//Following: functions for Simple Distributed File System based on Swim-Style Membership service
//TODO: put the following into another go file with the same package name, reformat the project
func putFile(localname, sdfsname string) error {
	//put local file to SDFS
	//select server every time
	updateLeader()

	//Connect to Master's RPC service
	rpcConn, err := rpc.DialHTTP("tcp", masterId.IP + RPC_PORT)
	if err != nil {
		fmt.Println("connect to master rpc service fail: ", err)
		return err
	}
	//TODO: is close() needed?
	defer rpcConn.Close()

	//Synchronous RPC call
	masterServiceArgs := MasterServiceArgs{myid, "PUT", nil, sdfsname}
	var reply ReplyToNode
	err = rpcConn.Call("RPC.MasterService", masterServiceArgs, &reply)
	if err != nil {
		fmt.Println("PUT RPC call to master fail: ", err)
		return err
	}

	replicas := reply.Dstids
	version := reply.Version
	resultsChan := make(chan UpdateResult)
	for _, replica := range replicas {
		go sendFile(localname, replica, version, sdfsname, resultsChan)
	}
	var updateResults []UpdateResult
	for i := 0; i < len(replicas); i++ {
		res := <-resultsChan
		fmt.Println("update result for", res.Id, ": ", res.Success)
		updateResults = append(updateResults, res)
	}

	updateArgs := MasterServiceArgs{myid, "UPDATE", updateResults, sdfsname}
	err = rpcConn.Call("RPC.MasterService", updateArgs, &reply)
	if err != nil {
		fmt.Println("UPDATE RPC call to master fail: ", err)
		return err
	}
	//TODO: should then send a finish signal after all sends are done, then the receiver can make sure the file is OK and tell master. In case of sender fail.
	return nil
}

func getFile(sdfsname, localname string) error {
	//get file from SDFS and store to local dir
	updateLeader()

	//Connect to Master's RPC service
	rpcConn, err := rpc.DialHTTP("tcp", masterId.IP + RPC_PORT)
	if err != nil {
		fmt.Println("GET connect to master rpc service fail: ", err)
		return err
	}
	defer rpcConn.Close()

	//Synchronous RPC call to MasterService
	masterServiceArgs := MasterServiceArgs{myid, "GET", nil, sdfsname}
	var reply ReplyToNode
	err = rpcConn.Call("RPC.MasterService", masterServiceArgs, &reply)
	if err != nil {
		fmt.Println("GET RPC call to master fail: ", err)
		return err
	}

	//Request for the sdfs file to the first replica
	if len(reply.Dstids) == 0 {
		fmt.Println("File currently not exist in SDFS!")
		return nil
	}
	replica := reply.Dstids[0]
	version := reply.Version
	nodeServiceConn, err := rpc.DialHTTP("tcp", replica.IP + RPC_PORT)
	if err != nil {
		fmt.Println("GET connect to node rpc service fail: ", err)
		return err
	}
	defer nodeServiceConn.Close()
	//At present, we assume getfile operation is reliable and will not crash
	localStorePath := fmt.Sprintf("%s%s", LOCAL_DIR_BASE, localname)
	nodeServiceArgs := NodeServiceArgs{myid, "GET", version, sdfsname, localStorePath, nil}
	err = nodeServiceConn.Call("RPC.NodeService", nodeServiceArgs, &reply)
	if err != nil {
		fmt.Println("GET file from replica fail: ", err)
		return err
	}

	return nil
}

func deleteFile(sdfsname string) error {
	//delete a SDFS file
	updateLeader()

	//Connect to Master's RPC service
	rpcConn, err := rpc.DialHTTP("tcp", masterId.IP + RPC_PORT)
	if err != nil {
		fmt.Println("DELETE connect to master rpc service fail: ", err)
		return err
	}
	defer rpcConn.Close()

	//Synchronous RPC call to MasterService
	masterServiceArgs := MasterServiceArgs{myid, "DELETE", nil, sdfsname}
	var reply ReplyToNode
	err = rpcConn.Call("RPC.MasterService", masterServiceArgs, &reply)
	if err != nil {
		fmt.Println("DELETE RPC call to master fail: ", err)
		return err
	}

	return nil
}

func lsFile(sdfsname string) error {
	//list all machine (VM) addresses where this file is currently being stored
	updateLeader()

	//Connect to Master's RPC service
	rpcConn, err := rpc.DialHTTP("tcp", masterId.IP + RPC_PORT)
	if err != nil {
		fmt.Println("DELETE connect to master rpc service fail: ", err)
		return err
	}
	defer rpcConn.Close()

	//Synchronous RPC call to MasterService
	masterServiceArgs := MasterServiceArgs{myid, "LS", nil, sdfsname}
	var reply ReplyToNode
	err = rpcConn.Call("RPC.MasterService", masterServiceArgs, &reply)
	if err != nil {
		fmt.Println("LS RPC call to master fail: ", err)
		return err
	}

	if reply.Replicas == nil {
		fmt.Println("Not find any replicas for ", sdfsname)
	} else {
		for ver, repList := range reply.Replicas {
			fmt.Printf("%d_%s: ", ver, sdfsname)
			for _, rep := range repList {
				fmt.Printf("VM%d:%s ", idxmap[rep]+1, rep)
			}
			fmt.Print("\n")
		}
	}

	return nil
}

func lsStoredFile() {
	//list all SDFS files currently being stored at this machine
	files, err := ioutil.ReadDir(fmt.Sprintf("%s%s", LOCAL_DIR_BASE, "sdfs/"))
	if err != nil {
		log.Fatal("list SDFS stored file fatal fail:", err)
	}

	for _, file := range files {
		fmt.Printf("%s ", file.Name())
	}
	fmt.Print("\n")

}

func gerVersions() {
	//get-versions sdfsfilename num-versions localfilename:
	//get all the last num-versions versions of the file into the localfilename(use delimiters to mark out versions).

}

//---------------------------------SDFS node service functions-----------------------------------------
func (t *RPC) NodeService(args *NodeServiceArgs, reply *ReplyToNode) error {
	//TODO: what error should be returned here???
	// This RPC function should be called synchronously to ensure ordering and no race on writing to MetaFile
	//TODO: Is the former statement true?
	//TODO: There might be a race between MasterService and the outside re-replication, figure it out
	//select server every time
	updateLeader()

	request := args.Request
	sdfsname := args.Sdfsname
	node := args.Myid
	version := args.Version
	remoteDir := args.LocalDir

	switch request {
	case "GET":
		fmt.Println("Receive a GET request from node, prepare to send file")
		localstore := fmt.Sprintf("%s%d_%s", SDFS_DIR_BASE, version, sdfsname)
		remoteNetDir := fmt.Sprintf("%s@%s:%s", USER, node.IP, remoteDir)
		cmd := exec.Command("sshpass", "-p", PASSWORD, "scp", localstore, remoteNetDir)

		fmt.Println("CMD executed: ", cmd)
		err := cmd.Run()
		if err != nil {
			fmt.Println("Send file to GET requester fail: ", err.Error())
			return err
		}
		return nil
	case "DELETE":
		for version >= 0 {
			filename := fmt.Sprintf("%s%d_%s", SDFS_DIR_BASE, version, sdfsname)
			if _, err := os.Stat(filename); !os.IsNotExist(err) {
				fmt.Println("file exist, delete it: ", filename)
				os.Remove(filename)
			}
			version--
		}
		fmt.Println("sdfs file ", sdfsname, " removed" )
	case "META":
		MetaFile = args.Meta
		fmt.Println("MetaFile in peer replica updated")
	}
	return nil
}

func nodeMessageServer() {

}

func nodeMessageHandler() {

}


//---------------------------------SDFS master functions-----------------------------------------------
//Following: functions for master metadata service and data replication
/*
func MasterServer() {
	sdfsMaster := new(SdfsMaster)
	rpc.Register(sdfsMaster)
	rpc.HandleHTTP()

	err := http.ListenAndServe(":" + string(MASTER_PORT), nil)
	if err != nil {
		fmt.Println("MasterServer RPC listening error: ", err.Error())
	}
}*/

func (t *RPC) MasterService(args *MasterServiceArgs, reply *ReplyToNode) error {
	//TODO: what error should be returned here???
	// This RPC function should be called synchronously to ensure ordering and no race on writing to MetaFile
	//TODO: Is the former statement true?
	//TODO: There might be a race between MasterService and the outside re-replication, figure it out
	//select server every time
	updateLeader()

	if myid.IP != masterId.IP {
		//TODO: maybe should reply that I'm not master and you need to resend RPC request!!!
		fmt.Println("Warning! I'm not master, please find someone else")
		return nil
	}
	request := args.Request
	sdfsname := args.Sdfsname
	node := args.Myid

	metaUpdateFlag := false

	switch request {
	case "PUT":
		fmt.Println("received a PUT request, processing...")
		replicasStates, ok := MetaFile[sdfsname]
		//TODO: need to check version with membership list and trigger re-replicate if needed
		replicas := assignReplicas(node, sdfsname)
		reply.Dstids = replicas
		newReplicasState := make(ReplicasState)
		for _, replica := range replicas {
			newReplicasState[replica] = false
		}
		if ok {
			//TODO: Does len of 2d array return correct result?
			reply.Version = len(replicasStates)
			replicasStates = append(replicasStates, newReplicasState)
		} else {
			reply.Version = 0
			replicasStates = append(replicasStates, newReplicasState)
		}
		MetaFile[sdfsname] = replicasStates
		metaUpdateFlag = true
		fmt.Print("Metafile:\n", MetaFile, "\n")
		//send file, update the file sending status after acknowledgement
		//TODO: update MetaFile only when received UPDATA signal?
	case "UPDATE":
		//SCP file transfer succeeds, master is allowed to update its metafile and re-replicate it
		fmt.Println("received an UPDATE request, updating...")
		updates := args.UpdateState

		newReplicaState := make(ReplicasState)
		for _, update := range updates {
			newReplicaState[update.Id] = update.Success
		}
		MetaFile[sdfsname][updates[0].Version] = newReplicaState
		metaUpdateFlag = true
		fmt.Print("Metafile:\n", MetaFile, "\n")
	case "GET":
		fmt.Println("received a GET request from node, prepare to send replica nid")
		//get the newest state of the sdfsfile
		allStates := MetaFile[sdfsname]
		if len(allStates) == 0 {
			//file does not exist in SDFS
			reply.Version = -1
			reply.Dstids = nil
		} else {
			curVer := len(allStates) - 1
			for curVer >= 0 {
				for nid, state := range allStates[curVer] {
					if state == true {
						reply.Version = curVer
						reply.Dstids = []Nid{nid}
						return nil
					}
				}
				curVer--
			}
			reply.Version = -1
			reply.Dstids = nil
		}
	case "DELETE":
		fmt.Println("received a DELETE request, prepare to send out delete request")
		allStates := MetaFile[sdfsname]
		highestVer := len(allStates) - 1
		if highestVer < 0 {
			fmt.Println("file never exists, no need to delete")
			return nil
		} else {
			curVer := highestVer//Actually here it is the highest version of file a node needs to delete
			nodeServiceArgs := NodeServiceArgs{myid, "DELETE", curVer, sdfsname, "", nil}
			deleteDsts := make(map[string]bool)
			for curVer >= 0 {
				for nid, state := range allStates[curVer] {
					if state == true {
						deleteDsts[nid.IP] = true
					}
				}
				curVer--
			}
			for dst := range deleteDsts {
				go rpcCall(dst, "RPC.NodeService", nodeServiceArgs)
			}
			delete(MetaFile, sdfsname)
			metaUpdateFlag = true
		}
	case "LS":
		fmt.Println("received a LS request, prepare to send out LS request")
		allStates := MetaFile[sdfsname]
		replicas := make(map[int][]string)
		highestVer := len(allStates) - 1
		if highestVer < 0 {
			reply.Replicas = nil
			return nil
		} else {
			curVer := highestVer
			for curVer >= 0 {
				for nid, state := range allStates[curVer] {
					if state == true {
						replicas[curVer] = append(replicas[curVer], nid.IP)
					}
				}
				curVer--
			}
		}
		reply.Replicas = replicas
	case "FAIL":
		fmt.Println("received a FAIL node", args.Myid.IP, "VM", idxmap[args.Myid.IP], ",prepare to re-replicate")

		nodeIdx := getIdx(node)
		if nodeIdx < 0 {
			return nil
		}
		repIdx := nodeIdx - 5
		if repIdx < 0 {
			repIdx = repIdx + len(membership)
		}
		repNode := membership[repIdx]
		//reReplicas := make(map[string]bool)
		for sdfsfilename, repsList := range MetaFile {
			for ver, reps := range repsList {
				for nid, state := range reps {
					if state == true {
						repDir := fmt.Sprintf("%s%d_%s", SDFS_DIR_BASE, ver, sdfsfilename)
						nodeServiceArgs := NodeServiceArgs{repNode, "GET", ver, sdfsfilename, repDir, nil}
						go rpcCall(nid.IP, "RPC.NodeService", nodeServiceArgs)
						break
					}
				}
			}
		}
	}

	if metaUpdateFlag == true {
		//Replicate the metafile to peers if there's a change
		fmt.Println("Replicating updated metafile")
		ips := assignMetaReplicas(myid)
		nodeServiceArgs := NodeServiceArgs{myid, "META", -1, "", "", MetaFile}
		for _, ip := range ips {
			if ip.IP != myid.IP {
				go rpcCall(ip.IP, "RPC.NodeService", nodeServiceArgs)
			}
		}
	}

	return nil
}

func rpcCall(ip, call string, args NodeServiceArgs) error {
	rpcConn, err := rpc.DialHTTP("tcp", ip + RPC_PORT)
	if err != nil {
		fmt.Println("rpcCall() error for ", call, ": ", err)
		return err
	}
	defer rpcConn.Close()

	//Synchronous call
	var reply ReplyToNode
	err = rpcConn.Call(call, args, &reply)
	if err != nil {
		fmt.Println("rpcCall() erroor for ", call , ": ", err)
		return err
	}
	fmt.Println(args.Request, " message sent success from master to peer node")
	return nil
}

func masterRPCCall(ip, call string, args MasterServiceArgs) error {
	rpcConn, err := rpc.DialHTTP("tcp", ip + RPC_PORT)
	if err != nil {
		fmt.Println("rpcCall() error for ", call, ": ", err)
		return err
	}
	defer rpcConn.Close()

	//Synchronous call
	var reply ReplyToNode
	err = rpcConn.Call(call, args, &reply)
	if err != nil {
		fmt.Println("rpcCall() erroor for ", call , ": ", err)
		return err
	}
	fmt.Println(args.Request, " message sent success from peer to master node")
	return nil
}

func assignReplicas(nodeid Nid, sdfsname string) []Nid{
	var replicaIds []Nid
	replicaStates, exist := MetaFile[sdfsname]
	if exist == true {
		for replicaId, state := range replicaStates[0] {
			if state == true {
				replicaIds = append(replicaIds, replicaId)
			}
		}
		return replicaIds
	}
	mu.Lock()
	nodeIdx := getIdx(nodeid)
	for i := 0; i < NUM_REPLICA; i++ {
		replicaIdx := nodeIdx - i
		if replicaIdx < 0 {
			replicaIdx = (replicaIdx + len(membership)) % len(membership)
		}
		replicaIds = append(replicaIds, membership[replicaIdx])
	}
	mu.Unlock()
	return replicaIds
}

func assignMetaReplicas(nodeid Nid) []Nid{
	//Assign four replica ids to "PUT" requester, including the id of the requester itself
	var replicaIds []Nid
	mu.Lock()
	nodeIdx := getIdx(nodeid)
	for i := 0; i < NUM_REPLICA; i++ {
		replicaIdx := nodeIdx - i
		if replicaIdx < 0 {
			replicaIdx = (replicaIdx + len(membership)) % len(membership)
		}
		replicaIds = append(replicaIds, membership[replicaIdx])
	}
	mu.Unlock()
	return replicaIds

}

/*func assignReplicas(nodeid Nid) []Nid{
	//Assign four replica ids to "PUT" requester, including the id of the requester itself
	var replicaIds []Nid
	mu.Lock()
	nodeIdx := getIdx(nodeid)
	for i := 0; i < NUM_REPLICA; i++ {
		replicaIdx := nodeIdx - i
		if replicaIdx < 0 {
			replicaIdx = (replicaIdx + len(membership)) % len(membership)
		}
		replicaIds = append(replicaIds, membership[replicaIdx])
	}
	mu.Unlock()
	return replicaIds

}*/

func masterMessageServer() {
	//server for receiving SDFS nodes' requests such as putFile, getFile and deleteFile

}

func masterMessageHandler() {
	//handle SDFS nodes' requests for server
}

func dataReplication() {

}



//---------------------------------SDFS utility functions-----------------------------------------------
//Following: utility functions for SDFS such as sending files, storing files and TCP message service
func updateLeader() {
	myidx := getIdx(myid)
	if myidx == len(membership) - 1 && myidx > 3 {
		if myid.IP != masterId.IP {
			fmt.Println("Leader is me now!")
		}
		masterId = myid
		if len(MetaFile) > 0 {
			//Not in initialization stage, should do a re-replicate here

		}
	} else if len(membership) - 1 > 0{
		masterId = membership[len(membership) - 1]
	}
}

func sendFile(localdir string, remoteId Nid, version int, sdfsname string, resultChan chan UpdateResult) {
	//send a local file to a remote node using unix scp command
	//if remoteId.IP == myid.IP {
	//directly copy file to sdfs path if remoteId equals myid
	//}
	//Use fmt.Sprintf() instead of "+" operator for string concatenation
	remotedir := fmt.Sprintf("%s%d_%s", SDFS_DIR_BASE, version, sdfsname)
	dst := fmt.Sprintf("%s@%s:%s", USER, remoteId.IP, remotedir)

	cmd := exec.Command("sshpass", "-p", PASSWORD, "scp", localdir, dst)

	fmt.Println("CMD executed: ", cmd)
	err := cmd.Run()

	//TODO: check what happens if scp file transfer fails
	if err != nil {
		fmt.Println("Send file error: ", err.Error())
		resultChan <- UpdateResult{remoteId, version, false}
		return
	}
	resultChan <- UpdateResult{remoteId, version, true}
	return
}

func sendMessage() error {

	return nil
}