.PHONY: all server clean

all: server

server:
	docker build -t sc-poc-generic-server:latest .

clean:
	docker rmi -f sc-poc-generic-server:latest