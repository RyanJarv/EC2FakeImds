.PHONY: build

build:
	sam build

deploy: build
	sam deploy

invoke: build
	sam local invoke --event ./event.json FakeImdsRouteFunction

#instance: deploy
#	aws ec2 run-instances --image-id ami-0b0f4c27376f8aa79 --instance-type t2.micro
