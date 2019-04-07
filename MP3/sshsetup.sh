mkdir ~/.ssh/
touch ~/.ssh/known_hosts
ssh-keyscan -H 172.22.156.115 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.158.115 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.154.116 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.156.116 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.158.116 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.154.117 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.156.117 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.158.117 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.154.118 >> ~/.ssh/known_hosts
ssh-keyscan -H 172.22.156.118 >> ~/.ssh/known_hosts