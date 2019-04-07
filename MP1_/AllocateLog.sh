#!/bin/bash


for i in 1 2 3 4 5 6 7 8 9 10
do
    sshpass -p "PASSWORD" scp /Users/garethfeng/Desktop/logs/vm$i.log mingren3@fa18-cs425-g35-$i.cs.illinois.edu:/home/mingren3/test
done
