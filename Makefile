build:
	GOOS=linux go build -o main main.go && zip deployment.zip main

deploy:
	terraform init && terraform apply
