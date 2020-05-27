WORKDIR ?= $(CURDIR)/../
OUTPUT ?= $(WORKDIR)/_output/

test:
	cd $(WORKDIR) && jenkins-jobs test -o $(OUTPUT) jobs

deploy:
	cd $(WORKDIR) && jenkins-jobs update jobs
