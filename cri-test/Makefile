build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o critest main.go

image: build
	docker build -t lilic/scope-cri:latest .
	docker push lilic/scope-cri:latest

minikube:
	minikube delete && minikube start --kubernetes-version=v1.10.0 --memory=4096 --bootstrapper=kubeadm --extra-config=kubelet.authentication-token-webhook=true --extra-config=kubelet.authorization-mode=Webhook --extra-config=scheduler.address=0.0.0.0 --extra-config=controller-manager.address=0.0.0.0 --container-runtime=cri-o --network-plugin=cni

cri:
	curl https://raw.githubusercontent.com/kubernetes/kubernetes/master/pkg/kubelet/apis/cri/runtime/v1alpha2/api.proto > ../cri-runtime/api.proto

gen:
	cd $(GOPATH)/src;protoc --proto_path=$(GOPATH)/src --gofast_out=plugins=grpc:. $(GOPATH)/src/github.com/weaveworks/scope/runtime/api.proto

.PHONY: all build image gen
