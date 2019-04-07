package main

import (
        "net/rpc"
        "fmt"
        "log"
        "os"
        "bufio"
        "time"
)

type Args struct {
        S1, S2 string
}

type Reply struct {
        S, COUNT string
}

type result struct {
        res, count, vm string
}

const vmlist = "vmlist.txt"
type typetime time.Time

var grepTime typetime

func main() {
        if len(os.Args) != 3 {
                fmt.Println("Usage: ", os.Args[0], "server")
                os.Exit(1)
        }

        vmAddresses, err := readLines(vmlist)
        if err != nil {
                log.Fatal("Error read lines:", err)
        }
        
        termargs := Args{os.Args[1], os.Args[2]}

        grepStart := time.Now()
        ch := make(chan result)
        for i := 0; i < len(vmAddresses); i++ {
                go grepClient(vmAddresses[i], termargs, ch)
        }

        m := make(map[string]string)

        for i := 0; i < len(vmAddresses); i++ {
                res := <-ch
                m[res.vm] = res.count
                fmt.Printf("%s\n", res.res)
                fmt.Printf("%s:%s\n", res.vm, res.count)                
        }

        
        for k, v := range m {
                fmt.Println("vm:", k, "count:", v)
        }
        t := time.Now()
        grepTime := t.Sub(grepStart)
        fmt.Println("Total time for grep:", grepTime)

}

func readLines(filename string) (out []string, err error) {
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

func grepClient(vm string, termargs Args, ch chan result) {
        client, err := rpc.DialHTTP("tcp", vm + ":1288")
        if err != nil {
                ch <- result{ " VM crashed", "0", vm}
                return
        }

        // Synchronous call
        var out Reply
        err = client.Call("Grep.Grepexec", termargs, &out)
        if err != nil {
                ch <- result{" Grep command illegal or pattern not found in this VM", "0", vm}
                return
        }

        ch <- result{ out.S, out.COUNT, vm}
        return
} 