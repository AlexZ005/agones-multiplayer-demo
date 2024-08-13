## Introduction

This project demonstrates a web application that enables users to launch game servers on Kubernetes using Agones for hosting multiplayer rooms. The application leverages the following technologies:

Agones (CRD for K8s), Go, NodeJS, MySQL, Websockets

## Project Structure

Here is a description of each file:

* **README.md**: This file provides an overview of the project and describes how to get started.
* **game-launcher**: web application written in Go for starting gameservers 
  * Dockerfile: This file contains the instructions for building the Docker image
  * game-server-create.go: This file contains the code that is used to start a game server instance


## Getting Started
To get started with the project, you will need to:

1. Install VirtualBox
1. Configure k8s cluster on minikube or Ubuntu
1. Download and configure docker and kubectl executables
1. Generate certificates

**NOTE:** docker locally is used only as a client to connect to a Virtual Machine using context

## Option 1: Create Kubernets cluster

1. Remove .minikube folder if exist!
1. Start K8s cluster using VirtualBox:
    ```
    minikube start --kubernetes-version v1.27.6 --driver=virtualbox  --no-vtx-check  --disk-size 64g
    ```
1. Get pods in kube-system namespace:
    ```
    minikube kubectl -- get pods --namespace kube-system
    ```
1. Create agones-system namespace:
    ```
    kubectl create namespace agones-system
    ```
1. Install Agones:
    ```
    kubectl apply --server-side -f https://raw.githubusercontent.com/googleforgames/agones/release-1.37.0/install/yaml/install.yaml
    ```
1. Wait until all pods are running:
    ```
    kubectl wait --namespace agones-system --for=condition=ready pods --all --timeout=120s
    ```


## Option 2: Configuring Ubuntu

1. Download Ubuntu 20.04 OVA image:
    ```
    https://cloud-images.ubuntu.com/releases/focal/release/ubuntu-20.04-server-cloudimg-amd64.ova  
    ```

1. Import to VirtualBox:
    ```
    * File > Import Appliance
    * Adjust CPU cores and RAM
    * Increase disk space in Tools
    * Set VM network as Bridge Adapter
    ```

1. Configure Networking and SSH:

    When Virtual Machine starts enter to the GRUB menu by holding Shift.
    Select advanced/recovery, after it boots choose 'root' from available options and press enter.  
    Configure a network connection:
    ```
    #https://askubuntu.com/questions/1442292/how-to-get-internet-on-qemu-cloudimg
    pubkey="place your ssh public key here"
    apt-get remove cloud-init -y && cd /etc/netplan && echo -e "network:\n  version: 2\n  renderer: networkd\n  ethernets:\n    enp0s3:\n       dhcp4: true" > 01-netcfg.yaml && ssh-keygen -A && mkdir ~/.ssh && echo $pubkey > ~/.ssh/authorized_keys && echo "IP Address: \4" >> /etc/issue && reboot
    ```
    **NOTE:** the command could be entered using Auto-Type functionality in KeePass. Optionally root password can be reset here, but since pubkey is used it is not necessarry.  

1. Adjust disk size
    ``` bash
    fdisk /dev/sda
    #Type following commands:
    d,1,n,1,p,enter,enter,N,w
    resize2fs /dev/sda1
    ```
    
1. Configure K8s  
    ``` bash
    cat << EOF > master.sh
    swapoff -a
    sudo apt-get update && sudo apt-get upgrade -y
    sudo apt install curl -y
    sudo apt-get install -y docker.io
    sudo apt-get install -y apt-transport-https ca-certificates curl gpg

    # Setting 1.27 version, update as after March 2024 Kubernetes package repositories (apt.kubernetes.io) were officially removed
    # https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/install-kubeadm/
    sudo mkdir -p -m 755 /etc/apt/keyrings
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.27/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.27/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list
    sudo apt-get update

    sudo apt-get install -y kubeadm kubelet kubectl
    sudo apt-mark hold kubelet kubeadm kubectl

    sudo kubeadm init --pod-network-cidr 192.168.0.0/16
    mkdir -p $HOME/.kube
    sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
    sudo chown $(id -u):$(id -g) $HOME/.kube/config
    # or under root
    # export KUBECONFIG=/etc/kubernetes/admin.conf
    # kubeadm token create --print-join-command
    EOF
    ```

    Run ./master.sh to create the Kubernetes cluster:  
    chmod +x ./master.sh  
    ./master.sh

    **Notes:**
    * The version will be picked automatically from the repo (as of writing: v1.27.16)
    * Alternatively specific version could be set as following if present in repository, for example:
    ```
    sudo apt-get install -y kubeadm=1.27.6-00 kubelet=1.27.6-00 kubectl=1.27.6-00
    ```
    * kubeadm init will say remote version is much newer: v1.30.3; falling back to: stable-1.27. Alternatively a specific version could be specified as following:  
    ```
    sudo kubeadm init --kubernetes-version 1.27.6 --pod-network-cidr 192.168.0.0/16
    ```

1. Setup K8s networking  
    ``` bash
    #ToDo: analyze if flanner or anything else is better:
    kubectl apply -f https://docs.projectcalico.org/manifests/calico.yaml
    #kubectl apply -f https://github.com/coreos/flannel/raw/master/Documentation/kube-flannel.yml
    kubectl taint nodes --all node-role.kubernetes.io/control-plane-
    ```
1. Apply Agones CRD  
    ``` bash
    kubectl create namespace agones-system
    kubectl apply --server-side -f https://raw.githubusercontent.com/googleforgames/agones/release-1.37.0/install/yaml/install.yaml
    kubectl wait --namespace agones-system --for=condition=ready pods --all --timeout=120s
    ```

## Download and configure docker and kubectl executables
Local executables should be configured for remote connection  
``` bash
docker context create server --docker "host=ssh://root@192.168.88.199"
docker context use server
scp root@192.168.88.199:/root/.kube/config %APPDATA%\..\..\.kube\config
```

**NOTE:** make sure to change server IP address inside config if using minikube  
>Line 5:     server: https://[MINIKUBE_IP]:8443   

# Game Launcher

Acess game-launcher folder and execute steps below.  

## Generate certificates
``` Bash
cd certs-ssl
openssl genrsa -des3 -out server.key 2048
openssl rsa -in server.key -out server.key
openssl req -sha256 -new -key server.key -out server.csr -subj "/CN=localhost"
openssl x509 -req -days 365 -in server.csr -signkey server.key -out server.crt
cd ..
```

## Copy config from .kube folder
copy %APPDATA%\..\..\.kube\config .\

**NOTE:** if kubernetes certificate data is used as files as it does in minikube, they also need to be copied and config file need to be modified to use them and Dockerfile need to be adjusted to copy these files into the image:  
```
cp %APPDATA%\..\..\.minikube\ca.crt .  
cp %APPDATA%\..\..\.minikube\profiles\minikube\client.crt .  
cp %APPDATA%\..\..\.minikube\profiles\minikube\client.key .  
```

## Build game-launcher
``` bash
set startTime=%time%
go mod init && go mod tidy
cd game-launcher
set GOARCH=amd64&& set GOOS=linux&& go build -v -o main
docker build -t game-launcher .
ssh root@192.168.88.199 ^
 ^"docker save game-launcher -o game-launcher.tar ^&^&^
 ctr -n=k8s.io images import game-launcher.tar^" && echo Start Time: %startTime% && echo Finish time: %time% && kubectl delete -f launcher.yaml && kubectl apply -f launcher.yaml
```