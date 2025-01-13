
buildapp:
	docker build -f dockerfile.app -t jbronson29/recipe:latest .
	docker push jbronson29/recipe:latest
buildcloudflare:
	docker build -f dockerfile.cloudflare -t jbronson29/cloudflare-tunnel:latest .
	docker push jbronson29/cloudflare-tunnel:latest	