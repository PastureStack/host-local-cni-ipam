.RECIPEPREFIX := >

GO ?= go

all: build

build:
>./scripts/build

test:
>./scripts/test

validate:
>./scripts/validate

.PHONY: all build test validate
