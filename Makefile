.PHONY: gen-env test

gen-env:
	./scripts/gen-env.sh

test:
	go test ./backend/...
	cd frontend && npm run check
