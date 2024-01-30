WORKDIR ?= $(CURDIR)/../
OUTPUT ?= $(WORKDIR)/_output/

test:
	cd $(WORKDIR) && jenkins-jobs test --allow-empty-variables -o $(OUTPUT) jobs

deploy:
	cd $(WORKDIR) && jenkins-jobs update --allow-empty-variables --delete-old jobs
