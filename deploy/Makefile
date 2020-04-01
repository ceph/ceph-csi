WORKDIR ?= $(CURDIR)/../
OUTPUT ?= $(WORKDIR)/_output/

test:
	jenkins-jobs test -o $(OUTPUT) $(WORKDIR)

deploy:
	jenkins-jobs update $(WORKDIR)
