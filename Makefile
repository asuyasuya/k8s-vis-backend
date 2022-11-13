.PHONY: prod
prod:
	docker rm api-prod-container
	docker rmi api-prod
	docker build -t api-prod --target prod .

.PHONY: push
push:
	docker tag api-prod asuyasuya/api-prod
	docker push asuyasuya/api-prod

.PHONY: run
run:
	docker run --name api-prod-container -p 8080:8080 -d api-prod