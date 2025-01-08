
buildimage:
	docker buildx build --platform linux/amd64,linux/arm64 . -t jbronson29/recipe:latest --push
	docker tag recipe:latest jbronson29/recipe:latest
	docker push jbronson29/recipe:latest