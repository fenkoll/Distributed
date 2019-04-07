package main

import (
	"encoding/json"
	"sort"
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


type Grep string


const (
	PORT = 6666
	INTRODUCER = "172.22.156.115" //vm1 is the introducer
	INTRO_PORT = 6667
	GREP_PORT = ":1288"
	MONITOR_NUM = 4
)

//var mutex = &sync.Mutex{}
var pb []Piggyback //Every time pb was sent, it first need to be checked for existing time
var ackbuf Request //Global variable for storing ACK for ping
var membership []Nid
var protocStart string //Start of protocol period TODO: need to be changed to proper timer type
var myid Nid
var idxmap map[string]int
var ch = make(chan Request)

func main() {
	go server()
	go swimDetect()
	go grepServer()

	initialize()
	printList(membership)
	printID(myid)
	//test()
	handleInput()

}

func handleInput() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Please enter number for commands:")
		fmt.Println("1-printmember; 2-printid; 3-join; 4-leave; or grep and press enter")
		fmt.Print("Enter text:")
		text, _ := reader.ReadString('\n')
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
		}
	}
}

func initialize() {
	fmt.Println("-------------Initializing--------------")
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
		log.Fatal()
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
			log.Fatal()
		}
		L:
		for idx := myidx + size - 2; idx <= myidx+size+3; idx++ {
			thisidx := idx % size
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
			time.Sleep(500 * time.Millisecond) //TODO: maybe change it to time.After format to be more accurate
			//ackbuf = <- ch
			select {
			case ackbuf = <-ch:
				updatePiggyback(ackbuf)
				break
			default:
				fmt.Println("no message received")
				//Delete non-ack node from membership list
				membership = append(membership[:thisidx], membership[thisidx+1:]...)
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
		idx := getIdx(message.Node)
		if idx >= 0 {
			//Delete node from membership list
			writeToFile(fmt.Sprintf("fail or leave:%s timestamp:%s\n", message.Node.IP, message.Node.Timestamp))
			membership = append(membership[:idx], membership[idx+1:]...)
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
			if time.Now().UnixNano() / 1e6 - item.Localtime < 5e3 {
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

func (t *Grep) Grepexec(args *Args, reply *Reply) error {
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

func grepServer() {
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

	grep := new(Grep)
	rpc.Register(grep)
	rpc.HandleHTTP()

	err = http.ListenAndServe(GREP_PORT, nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func grepClient(vm string, termargs Args, resultch chan result) {
	client, err := rpc.DialHTTP("tcp", vm + GREP_PORT)
	if err != nil {
		resultch <- result{ " VM crashed", "0", vm}
		return
	}

	// Synchronous call
	var out Reply
	err = client.Call("Grep.Grepexec", termargs, &out)
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
		log.Fatal(err)
	}
}
