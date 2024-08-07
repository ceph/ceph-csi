WORKDIR ?= $(CURDIR)/../
OUTPUT ?= $(WORKDIR)/_output/

test:
	cd $(WORKDIR) && jenkins-jobs --allow-empty-variables test -o $(OUTPUT) jobs

deploy:
	cd $(WORKDIR) && jenkins-jobs --allow-empty-variables update --jobs-only --delete-old jobs
