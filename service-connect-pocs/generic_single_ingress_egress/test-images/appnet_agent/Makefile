.PHONY: all pinpong clean

all: pinpong

pinpong:
	docker build -t sc-poc-generic-server-appnet-agent:latest .

clean:
	docker rmi -f sc-poc-generic-server-appnet-agent:latest