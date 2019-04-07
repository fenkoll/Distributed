package main

import (
	"fmt"
	"os/exec"
)

func main() {
	localfile := "./README.md"
	remoteDir := "zz40@172.22.156.115:/home/zz40/mp3/sshpasstest"

	cmd := exec.Command("sshpass", "-p", "Zhzh#96zzl", "scp", localfile, remoteDir)
	fmt.Println("cmd: ", cmd)
	err := cmd.Run()
	if err != nil {
		fmt.Println("err:", err)
	}

}