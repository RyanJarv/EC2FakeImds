.PHONY: build

build:
	sam build

deploy: build
	sam deploy

invoke: build
	sam local invoke --event ./event.json FakeImdsRouteFunction

debug-build:
	cd FakeImdsRoute; GOOS=linux GOARCH=amd64 go build -gcflags "all=-N -l"  -o ../.aws-sam/build/FakeImdsRouteFunction/FakeImdsRoute .
debug: debug-build
	DEBUGGER_ARGS="--accept-multiclient --headless=true" echo {} | sam local invoke -d 2345 --debugger-path /opt/linux/dlv/ --debug-args "-delveAPI=2" --event ./event.json FakeImdsRouteFunction  

#instance: deploy
#	aws ec2 run-instances --image-id ami-0b0f4c27376f8aa79 --instance-type t2.micro
