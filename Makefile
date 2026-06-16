.PHONY: build run clean start down remove logs restart status test backup restore clear

build:
	docker compose build

run: build
	./oge-bot

clean:
	rm -f oge-bot

start:
	docker compose up -d --build

down:
	docker compose down

remove:
	docker compose down -v --rmi local

logs:
	docker compose logs -f

restart:
	docker compose restart

status:
	docker compose ps

test:
	go test ./... -count=1 -v

backup:
	@mkdir -p backups
	@zip -r backups/backup-$$(date +%Y%m%d-%H%M%S).zip data/
	@echo "Backup saved to backups/"

restore:
	@if [ ! -f "$(FILE)" ]; then echo "Usage: make restore FILE=backups/backup-XXXXXX.zip"; exit 1; fi
	@mkdir -p data
	@unzip -o $(FILE) -d .
	@echo "Restored from $(FILE)"

clear:
	docker compose down --rmi all 2>/dev/null; true
	@images=$$(docker images --format '{{.Repository}}:{{.Tag}}' | grep -i 'oge-request\|oge-bot'); \
	if [ -n "$$images" ]; then echo "$$images" | xargs docker rmi -f 2>/dev/null; fi
	@echo "All project images removed"
