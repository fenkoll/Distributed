# Client.go and Server.go Setup

1. Clone this repo to your local computer

```bash
git clone https://gitlab-beta.engr.illinois.edu/mingren3/MP1.git
```

2. Download vm1.log-vm10.log from <https://tinyurl.com/y94rk9nf> to current path

3. Run setup.sh to send the git repo as well as logs for unit test to every remote VMs

   ```bash
   chmod +x ./setup.sh
   ./setup.sh
   ```

4. SSH to every VM, run server.go by

   ```bash
   go run server.go
   ```

5. SSH to any one of the VM to run the client by

   ```bash
   go run client.go
   ```




# MP1 Unit Test Setup

1. Clone this repo to your local computer

```bash
git clone https://gitlab-beta.engr.illinois.edu/mingren3/MP1.git
```

2. Download log file 01-04(each about 60MB) for unit test to your current folder from [this google drive][https://drive.google.com/drive/folders/17DHHL8eqpAnK9hQtATouNvWl12km4oN4]
3. Run testsetup.sh to send the git repo as well as logs for unit test to every remote VMs

```bash
chmod +x ./testsetup.sh
./testsetup.sh
```

4. SSH to every VM, run server.go by

```bash
go run server.go
```

5. SSH to any one of the VM to run the unit test by

```bash
go test
```



