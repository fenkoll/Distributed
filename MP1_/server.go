
package main

import (
        "fmt"
        "net/rpc"
        "net/http"
        "os/exec"
)

type Args struct {
        S1, S2 string
}

type Reply struct {
        S, COUNT string
}

type Grep string

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

func main() {

        grep := new(Grep)
        rpc.Register(grep)
        rpc.HandleHTTP()

        err := http.ListenAndServe(":1288", nil)
        if err != nil {
                fmt.Println(err.Error())
        }
}