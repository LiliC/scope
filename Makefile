build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o critest main.go

image: build
	docker build -t lilic/scope-cri:latest .
	docker push lilic/scope-cri:latest

minikube:
	minikube delete && minikube start --kubernetes-version=v1.10.0 --memory=4096 --bootstrapper=kubeadm --extra-config=kubelet.authentication-token-webhook=true --extra-config=kubelet.authorization-mode=Webhook --extra-config=scheduler.address=0.0.0.0 --extra-config=controller-manager.address=0.0.0.0 --container-runtime=cri-o --network-plugin=cni

.PHONY: all build image
