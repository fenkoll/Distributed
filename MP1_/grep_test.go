package main

import (
        "testing"
        "log"
        "os/exec"
        "fmt"
        "time"
)

const NUMVM = 4

func TestGrep(t *testing.T) {
        repTest("12", 5, t)
        repTest("9.\\......", 5, t)
        repTest("234829", 5, t)
        repTest("1800.", 5, t)
        repTest("ted", 5, t)
        repTest("jack", 5, t)

}

func repTest(arg string, rep int, t *testing.T) {
        var greptimes []time.Duration
        for i := 0; i < rep; i++ {
                greptime := testTask(arg, t)
                greptimes = append(greptimes, greptime)
        }
        for i := 0; i < rep; i++ {
                fmt.Println( "Greptime for \"", arg, "\":", i, greptimes[i])
        }
}

func testTask(arg string, t *testing.T) time.Duration {

        vmAddresses, err := readLines(vmlist)
        if err != nil {
                log.Fatal("Error read lines:", err)
        }

        expected_map := make(map[string]string)
        for i := 0; i < NUMVM; i++ {
                logfile := fmt.Sprintf("%s%s%s", "testvm0", fmt.Sprintf("%d", i+1),".log")
                count, err := exec.Command("grep", "-c", arg, logfile).Output()
                if err != nil {
                        fmt.Println("exec.Command error or no result:", err.Error())
                }
                expected_map[vmAddresses[i]] = string(count)
        }

        //expected_total, err := exec.Command("grep", "-c", "23452", "vm" + string(i+1) + ".log").Output()

        start := time.Now()
        ch := make(chan result)
        for i := 0; i < NUMVM; i++ {
                go grepClient(vmAddresses[i], Args{arg, fmt.Sprintf("%s%s%s", "testvm0", fmt.Sprintf("%d", i+1),".log")}, ch)
        }

        result_m := make(map[string]string)
        //Here we cannot use a enhanced for loop
        for i := 0; i < NUMVM; i++ {
        //for res := range ch {
                res := <-ch
                result_m[res.vm] = res.count
                //fmt.Printf("%s\n", res.res)
                //fmt.Printf("%s:%s\n", res.vm, res.count) 
        }

        end := time.Now()
        
        mygreptime := end.Sub(start)
        fmt.Println("Total time for grep:", mygreptime)

        for k, v := range result_m {
                fmt.Println("For VM IP", k, "expected", expected_map[k], "got", result_m[k])
                if v != expected_map[k] && (v!="0" && expected_map[k]!="0"){
                        t.Error(
                                "For", k,
                                "expected", expected_map[k],
                                "got", result_m[k],
                                )
                }
        } 
        return mygreptime
}