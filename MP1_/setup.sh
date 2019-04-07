#!/bin/bash


for i in 01 02 03 04 05 06 07 08 09 10
do

    sshpass -p "$PASSWORD" scp /Users/zhengliangzhu/Documents/UIUC/courses/cs425/mp1/logs/vm$i.log zz40@fa18-cs425-g35-$i.cs.illinois.edu:/home/zz40/demo/MP1/vm.log
done
