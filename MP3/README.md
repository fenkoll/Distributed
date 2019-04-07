# CS425-MP3

1. You need to have *sshpass* command tool installed for using this MP. On a CS425 VM(centos/fedora), run following to install:

```bash
sudo yum install sshpass
```

2. You need to set VM IPs to known hosts for using *sshpass*. On every VM, run the following command for setting up:

```bash
chmod +x ./sshsetup.sh
./sshsetup.sh
```

3. Clone this repo to your local computer

```git clone https://gitlab-beta.engr.illinois.edu/mingren3/MP1.git
git clone https://gitlab.engr.illinois.edu/zz40/cs425-mp3.git
```

4. SSH to every VM, run membership.go by

```
go run membership.go
```

​	And enter commands for print list, print id, join, leave, put, get, delete and grep.

5. SSH to VM1 to run the introducer by

```
go run introducer.go
```

