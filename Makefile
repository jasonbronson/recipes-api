
# Setup buildx builder for multi-platform builds
setup-buildx:
	docker buildx create --name multiarch --use || docker buildx use multiarch
	docker buildx inspect --bootstrap

buildapp: setup-buildx
	docker buildx build --platform linux/amd64,linux/arm64 -f dockerfile.app -t jbronson29/recipes:latest --push .
buildcloudflare:
	docker build -f dockerfile.cloudflare -t jbronson29/cloudflare-tunnel:latest .
	docker push jbronson29/cloudflare-tunnel:latest

# Push recipe image with custom tag
# Usage: make push-recipe TAG=your-tag-name
push-tag:
	docker push jbronson29/recipes:$(TAG)

# Build and push recipe image with custom tag (also pushes latest)
# Usage: make build-push-recipe TAG=your-tag-name
build-push-tag:
	docker buildx build --platform linux/amd64,linux/arm64 -f dockerfile.app -t jbronson29/recipes:$(TAG) --push .
	docker buildx build --platform linux/amd64,linux/arm64 -f dockerfile.app -t jbronson29/recipes:latest --push .

# Docker Compose commands using docker-stack.yml
up:
	docker-compose -f docker-stack.yml up -d

down:
	docker-compose -f docker-stack.yml down

# Clean up everything and force fresh pull
clean-all:
	docker-compose -f docker-stack.yml down
	docker rmi jbronson29/recipes:latest || true
	docker system prune -f

# Clean and rebuild everything
fresh-deploy: clean-all buildapp up

logs:
	docker-compose -f docker-stack.yml logs -f

ps:
	docker-compose -f docker-stack.yml ps

restart:
	docker-compose -f docker-stack.yml restart

# Docker Stack commands for swarm mode
deploy:
	docker stack deploy -c docker-stack-swarm.yml recipes

remove:
	docker stack remove recipes	
deploy:
	rsync -av --exclude data --exclude .env --exclude .git --exclude tmp \
		/Volumes/development/_personalprojects/cooking.bronson.dev/api/ \
		root@159.89.240.60:/root/recipes/
