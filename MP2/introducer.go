package main

import (
	"bufio"
	"encoding/json"
	"net"
	"sort"
	"time"
	"fmt"
	"log"
	"os"
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

/*type Request struct {
	Action string
	Content string
}*/
const (
	NODE_PORT = 6666
	MY_IP = "172.22.156.115"
	MY_PORT = 6667
	MONITOR_NUM = 4
)

var idxmap =  make(map[string]int)
var curnodes []Nid
var timestamp int64

func main() {
	err := readLines("vmlist.txt")
	if err != nil {
		log.Fatal("Error read lines:", err)
	}
	introduce()
}


func readLines(filename string) (error) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("Open file error:", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	i := 0
	for scanner.Scan() {
		idxmap[scanner.Text()]=i
		i++
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("File scanner error:", err)
	}

	return err
}

func insert(newnode Nid) {
	var idxlist []int
	for _, node := range curnodes {
		idxlist = append(idxlist, idxmap[node.IP])
	}
	insert := sort.Search(len(idxlist), func(i int) bool { return idxlist[i] > idxmap[newnode.IP] })
	fmt.Println("insert pos:", insert)

	if len(curnodes) == 0 {
		curnodes = []Nid{newnode}
	} else {
        curnodes = append(curnodes[:insert], append([]Nid{newnode}, curnodes[insert:]...)...)
    }

	/*else if insert == 0 {
		curnodes = append([]Nid{newnode}, curnodes[insert+1:]...)
	} else if insert == len(curnodes) {
		curnodes = append(curnodes[:insert], newnode)
	} else {
		curnodes = append(curnodes[:insert], append([]Nid{newnode}, curnodes[insert+1:]...)...)
		}*/
	}

func introduce() {
	fmt.Println("Server start!")
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", MY_PORT))
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for {
		handleNodeRequest(conn)
	}

}

func handleNodeRequest(conn *net.UDPConn) {
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
	case "JOIN":
		remoteAddr := fmt.Sprintf("%s", addr.IP)
		if remoteAddr == "127.0.0.1" {
			remoteAddr = MY_IP
		}
		fmt.Println("Node join request:", remoteAddr)
		newNode := Nid{remoteAddr, time.Now().UnixNano() / 1e6}
		//fmt.Println("connecting remote address for ping", remoteAddr)
		for _, node := range curnodes {
			response := Request{"JOIN", nil, []Nid{newNode}}
			sendRequst(response, node.IP, NODE_PORT)
		}

		insert(newNode)
		addr.Port = NODE_PORT
		response := Request{"INIT", nil, curnodes}
		jsonResponse, err := json.Marshal(&response)
		if err != nil {
			log.Print(err)
			break
		}
		conn.WriteToUDP(jsonResponse, addr)
		printList(curnodes)



	case "FAIL", "LEAVE":
		remoteAddr := fmt.Sprintf("%s", addr.IP)
		if remoteAddr == "127.0.0.1" {
			remoteAddr = MY_IP
		}
		fmt.Println("FAIL/LEAVE received from: ", remoteAddr)

		if len(request.Node) != 1 {
			fmt.Println("Error: FAIL/LEAVE request includes no nodes or multiple nodes")
			break
		}
		leaveidx := getIdx(request.Node[0])
		if leaveidx < 0 {
			fmt.Println("FAIL node already deleted, do nothing.")
			break
		}
		//Remove the FAIL or LEAVE node from current nodes
		curnodes = append(curnodes[:leaveidx], curnodes[leaveidx+1:]...)

		printList(curnodes)

	}
}

func getIdx(id Nid) int {
	for idx, node := range curnodes {
		if node.IP == id.IP && node.Timestamp == id.Timestamp {
			return idx
		}
	}
	return -1
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
	fmt.Println("Request sent")
	return err
}

func printList(msList []Nid) {
	fmt.Println("-------------Printing membership list--------------")
	for _, vm := range msList {
		fmt.Println("VM:", vm.IP, " Timestamp:", vm.Timestamp)
	}
}