.PHONY: test deploy-docs

docs/_build/html/.git:
	git worktree add -f --checkout docs/_build/html gh-pages
	
docs: docs/* docs/_build/html/.git
	$(MAKE) -C docs html

test:
	go test ./pkg/...

deploy-docs: docs
	cd docs/_build/html ; git add --all && git commit -m 'docs: build' -m '[skip ci]' && git push
