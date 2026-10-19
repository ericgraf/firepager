# FirePager V2 Makefile

# --- Configuration ---
BACKEND_DIR = backend
CLIENT_DIR = client
FRONTEND_DIR = frontend
K8S_DIR = k8s
KIND_CLUSTER_NAME = firepager

# --- Help ---
.PHONY: help
help:
	@echo "FirePager V2 Management Commands:"
	@echo "  make build             - Build all Docker images"
	@echo "  make test              - Run all tests (backend, client, frontend)"
	@echo "  make simulate          - Run the client message simulator"
	@echo "  make simulate-random   - Run the client message simulator in random mode indefinitely"
	@echo "  make simulate-ollama   - Generate realistic raw pager calls with Ollama (gemma3:1b)"
	@echo ""
	@echo "  Development (Watch & Deploy):"
	@echo "    make watch           - Watch for changes in all components and auto-deploy to Kind"
	@echo "    make watch-backend   - Watch backend only"
	@echo "    make watch-frontend  - Watch frontend only"
	@echo "    make watch-client    - Watch client only"
	@echo ""
	@echo "  Kubernetes (Kind):"
	@echo "    make k8s-deploy      - Deploy infrastructure and apps to the current K8s context"
	@echo "    make k8s-delete      - Remove all resources from the K8s context"
	@echo "    make k8s-forward     - Port-forward services for local access (RabbitMQ, Postgres, Frontend)"
	@echo "    make k8s-logs-client - Follow logs for the firepager-client"
	@echo ""
	@echo "  Individual Component Targets:"
	@echo "    make backend-test    - Run Go backend tests"
	@echo "    make client-test     - Run Python client tests"
	@echo "    make frontend-test   - Run Frontend tests (Vite/TS)"

# --- Docker ---
.PHONY: build
build:
	@echo "Building Docker images..."
	docker build -t firepager-backend:latest $(BACKEND_DIR)
	docker build -t firepager-client:latest $(CLIENT_DIR)
	docker build -t firepager-frontend:latest $(FRONTEND_DIR)

# --- Development Watchers ---
.PHONY: watch watch-backend watch-frontend watch-client watch-k8s
watch:
	@echo "Starting watchers for all components (Ctrl+C to stop)..."
	@parallel --tag --halt now,fail=1 ::: \
		"make watch-backend" \
		"make watch-frontend" \
		"make watch-client" \
		"make watch-k8s"

watch-backend:
	@echo "Watching backend..."
	@find $(BACKEND_DIR) -type f -not -path '*/.*' | entr -r make backend-update

watch-frontend:
	@echo "Watching frontend..."
	@find $(FRONTEND_DIR) -type f -not -path '*/.*' -not -path '*/node_modules/*' -not -path '*/dist/*' | entr -r make frontend-update

watch-client:
	@echo "Watching client..."
	@find $(CLIENT_DIR) -type f -not -path '*/.*' -not -path '*/__pycache__/*' | entr -r make client-update

watch-k8s:
	@echo "Watching k8s manifests..."
	@find $(K8S_DIR) -type f -not -path '*/.*' | entr -r make k8s-deploy

.PHONY: backend-update frontend-update client-update
backend-update:
	@echo "Updating Backend..."
	docker build -t firepager-backend:latest $(BACKEND_DIR)
	kind load docker-image firepager-backend:latest --name $(KIND_CLUSTER_NAME)
	kubectl rollout restart deployment firepager-backend firepager-processor

frontend-update:
	@echo "Updating Frontend..."
	docker build -t firepager-frontend:latest $(FRONTEND_DIR)
	kind load docker-image firepager-frontend:latest --name $(KIND_CLUSTER_NAME)
	kubectl rollout restart deployment firepager-frontend

client-update:
	@echo "Updating Client..."
	docker build -t firepager-client:latest $(CLIENT_DIR)
	kind load docker-image firepager-client:latest --name $(KIND_CLUSTER_NAME)
	kubectl rollout restart deployment firepager-client

# --- Tests ---
.PHONY: test backend-test client-test frontend-test
test: backend-test client-test frontend-test

backend-test:
	@echo "Testing Backend..."
	cd $(BACKEND_DIR) && go test ./...

client-test:
	@echo "Testing Client..."
	python3 -m unittest discover -s $(CLIENT_DIR) -p 'test_*.py'

.PHONY: calibrate-sdr
calibrate-sdr:
	@echo "Example: make calibrate-sdr STATIONS='local=99.9 another=101.1'"
	@set -e; for station in $(STATIONS); do \
		python3 $(CLIENT_DIR)/sdr_calibrate.py --station "$$station" --gain "$${SDR_GAIN:-49}"; \
	done

frontend-test:
	@echo "Testing Frontend..."
	cd $(FRONTEND_DIR) && npm test || echo "No tests configured for frontend."

# --- Simulator ---
.PHONY: simulate simulate-random simulate-ollama
simulate:
	@echo "Running Simulator (Single Test Message)..."
	python3 $(CLIENT_DIR)/simulator.py

simulate-random:
	@echo "Running Simulator (Random mode, every 5 seconds)..."
	python3 $(CLIENT_DIR)/simulator.py --random --count 0 --delay 5

simulate-ollama:
	@echo "Running Simulator with Ollama-generated raw pager calls..."
	python3 $(CLIENT_DIR)/simulator.py --ollama --ollama-model gemma3:1b --count 0 --delay 5

# --- Kubernetes ---
.PHONY: k8s-deploy k8s-delete k8s-forward k8s-logs-client
k8s-deploy:
	@echo "Deploying to Kubernetes via Kustomize..."
	kubectl apply -k $(K8S_DIR)

k8s-delete:
	@echo "Deleting from Kubernetes..."
	kubectl delete -k $(K8S_DIR)

k8s-forward:
	@echo "Port-forwarding services (Ctrl+C to stop)..."
	@echo " Frontend: http://localhost:5173"
	@echo " RabbitMQ: http://localhost:15672 (Management) and 5672 (AMQP)"
	@echo " Postgres: localhost:5432"
	@parallel --tag --halt now,fail=1 ::: \
		"kubectl port-forward svc/firepager-frontend 5173:80" \
		"kubectl port-forward svc/firepager-rmq 5672:5672 15672:15672" \
		"kubectl port-forward svc/firepager-db 5432:5432"

k8s-logs-client:
	@echo "Following logs for firepager-client..."
	kubectl logs -l app=firepager-client -f
