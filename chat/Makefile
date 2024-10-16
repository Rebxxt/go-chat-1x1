migrate:
	cd migrations && GOOSE_DRIVER=postgres GOOSE_DBSTRING="postgresql://postgres:password@localhost:9980/postgres" goose up

unmigrate:
	cd migrations && GOOSE_DRIVER=postgres GOOSE_DBSTRING="postgresql://postgres:password@localhost:9980/postgres" goose down