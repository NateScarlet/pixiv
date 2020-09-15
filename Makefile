.PHONY: test deploy-docs default

default: docs

docs/_build/html/.git:
	rm -rf docs/_build/html
	git worktree add docs/_build/html -B gh-pages

docs: export SPHINXOPTS=-a
docs: docs/* docs/_build/html/.git
	$(MAKE) -C docs html

test:
	go test ./pkg/...

deploy-docs: docs
	cd docs/_build/html ; git add --all && git commit --amend -m 'docs: build' -m '[skip ci]' && git push -f
