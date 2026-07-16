# Discurd developer workflow. Run `make` (or `make help`) for the target list.
# Recipes shell out to `docker compose`; a POSIX sh (e.g. Git Bash) is assumed.

COMPOSE     = docker compose
COMPOSE_ALL = docker compose -f docker-compose.yml -f docker-compose.scale.yml
LOGS_SVC   ?=

CURL_CODE = curl -s -o /dev/null -m 5 -w '%{http_code}'

.PHONY: help up down down-v build logs ps seed scale cqlsh health clean

help: ## Show this help
	@grep -E '^[a-zA-Z][a-zA-Z0-9_-]*:.*## ' Makefile | \
		awk 'BEGIN {FS = ":.*## "} {printf "  %-8s %s\n", $$1, $$2}'

up: ## Build images and start the full stack in the background
	$(COMPOSE) up -d --build

down: ## Stop and remove containers (data volumes are kept)
	$(COMPOSE_ALL) down --remove-orphans

down-v: ## Stop and remove containers AND volumes (destroys all data)
	$(COMPOSE_ALL) down -v --remove-orphans

build: ## Build all images without starting anything
	$(COMPOSE) build

logs: ## Follow logs; narrow to one service with LOGS_SVC=<name> (e.g. make logs LOGS_SVC=api)
	$(COMPOSE_ALL) logs -f --tail=100 $(LOGS_SVC)

ps: ## Show container status (includes scale-overlay services when running)
	$(COMPOSE_ALL) ps

seed: ## Load demo data through the public HTTP API (stack must be up)
	$(COMPOSE) exec api /app/seed

scale: ## Start the scaled stack: 3-node Scylla, 3x api, 2x gateway, 2x web
	$(COMPOSE_ALL) up -d --build

cqlsh: ## Open an interactive cqlsh shell on the Scylla seed node
	$(COMPOSE) exec scylla cqlsh

health: ## Probe Traefik-routed endpoints and service UIs (prints HTTP status codes)
	@printf '%-60s' 'web        http://localhost/                    (expect 200)'; $(CURL_CODE) http://localhost/ || printf 'unreachable'; echo
	@printf '%-60s' 'api        http://localhost/api/v1/users/@me    (expect 401)'; $(CURL_CODE) http://localhost/api/v1/users/@me || printf 'unreachable'; echo
	@printf '%-60s' 'gateway    http://localhost/ws                  (expect 400)'; $(CURL_CODE) http://localhost/ws || printf 'unreachable'; echo
	@printf '%-60s' 'files      http://localhost/files/avatars/      (expect 403)'; $(CURL_CODE) http://localhost/files/avatars/ || printf 'unreachable'; echo
	@printf '%-60s' 'traefik    http://localhost:8090/ping           (expect 200)'; $(CURL_CODE) http://localhost:8090/ping || printf 'unreachable'; echo
	@printf '%-60s' 'prometheus http://localhost:9090/-/healthy      (expect 200)'; $(CURL_CODE) http://localhost:9090/-/healthy || printf 'unreachable'; echo
	@printf '%-60s' 'grafana    http://localhost:3000/api/health     (expect 200)'; $(CURL_CODE) http://localhost:3000/api/health || printf 'unreachable'; echo
	@printf '%-60s' 'minio      http://localhost:9001/               (expect 200)'; $(CURL_CODE) http://localhost:9001/ || printf 'unreachable'; echo
	@printf '%-60s' 'nats       http://localhost:8222/healthz        (expect 200)'; $(CURL_CODE) http://localhost:8222/healthz || printf 'unreachable'; echo

clean: ## down-v plus remove locally built images and orphans
	$(COMPOSE_ALL) down -v --remove-orphans --rmi local
