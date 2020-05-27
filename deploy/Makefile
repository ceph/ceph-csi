WORKDIR ?= $(CURDIR)/../
OUTPUT ?= $(WORKDIR)/_output/

test:
	jenkins-jobs test -o $(OUTPUT) $(WORKDIR)/jobs

deploy:
	jenkins-jobs update $(WORKDIR)/jobs
