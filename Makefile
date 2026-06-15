.PHONY: build run clean docker

build:
	go build -ldflags="-s -w" -o oge-bot .

run: build
	./oge-bot

clean:
	rm -f oge-bot

docker:
	docker compose up -d --build

docker-stop:
	docker compose down
