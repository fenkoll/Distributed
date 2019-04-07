#!/bin/bash


for i in 01 02 03 04
do

    sshpass -p "$PASSWORD" scp /Users/zhengliangzhu/Documents/UIUC/courses/cs425/mp1/logs/testvm$i.log zz40@fa18-cs425-g35-$i.cs.illinois.edu:/home/zz40/demo/MP1/testvm$i.log
done
