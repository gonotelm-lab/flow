.PHONY: run-server run-web web-install

run-server:
	@$(MAKE) -C server run-server

run-web:
	@cd web && npm run dev

web-install:
	@cd web && npm install
