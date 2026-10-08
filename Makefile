# The directory to serve and the port; another port than the default
# 6419 keeps out of the way of a gh-mini already running
DIR ?= internal/markdown/testdata
PORT ?= 7419

.PHONY: dev
dev:
	air -- -p $(PORT) $(DIR)
