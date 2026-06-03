# Makefile to build the project
GO=go
LINT=golangci-lint
LINTOPTS=
TEST_TAGS=
COVERAGE=-coverprofile=coverage.txt -covermode=atomic

all: tidy test lint
travis-ci: tidy test-cov lint

test:
	${GO} test ./... ${TEST_TAGS}

test-cov:
	${GO} test ./... ${TEST_TAGS} ${COVERAGE}

test-int:
	${GO} test ./... -tags=integration

test-int-cov:
	${GO} test ./... -tags=integration ${COVERAGE}

lint:
	${LINT} run --build-tags=integration ${LINTOPTS}

tidy:
	${GO} mod tidy