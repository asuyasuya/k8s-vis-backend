.PHONY: prod
prod:
	docker build -t api-prod --target prod .