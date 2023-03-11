#
#grpc:
#	go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
#	go get -u google.golang.org/grpc
#

service.pb.go: service.proto
	protoc service.proto --go_out=plugins=grpc:. --go_opt=paths=source_relative

force:
	rm -f service.pb.go
	make service.pb.go
