# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

## [0.5.0](https://github.com/NateScarlet/pixiv/compare/v0.4.2...v0.5.0) (2021-01-09)


### ⚠ BREAKING CHANGES

* **artwork:** use int64 for resolution search option
* **artwork:** add SearchMode type
* **artwork:** add ContentRating type
* **artwork:** add Order type
* **artwork:** rename SearchOptionSearchMode -> SearchOptionMode
* **artwork:** rename SearchOptionMode -> SearchOptionContentRating
* **artwork:** rename SearchOptionResolutionRatio -> SearchOptionResolution

### Features

* **artwork:** add ContentRating type ([7c74e2d](https://github.com/NateScarlet/pixiv/commit/7c74e2de075dc56d2869e8019742638e72070588))
* **artwork:** add Order type ([2df58b8](https://github.com/NateScarlet/pixiv/commit/2df58b8940bf6f8cc2da48087e4331c46ac2833b))
* **artwork:** add SearchMode type ([18ab9a0](https://github.com/NateScarlet/pixiv/commit/18ab9a04c756735c971e32bad0a93a5f4aab7b8a))
* **artwork:** rename SearchOptionMode -> SearchOptionContentRating ([f7ff4b4](https://github.com/NateScarlet/pixiv/commit/f7ff4b49e0901085e87ec321cbe602cba19d68df))
* **artwork:** rename SearchOptionResolutionRatio -> SearchOptionResolution ([c1214d6](https://github.com/NateScarlet/pixiv/commit/c1214d6a827d1c8b169da9864f411252002af041))
* **artwork:** rename SearchOptionSearchMode -> SearchOptionMode ([bd8a57c](https://github.com/NateScarlet/pixiv/commit/bd8a57c5d6bc620edc49c15c5c3bdc8cfdc9fadf))
* **artwork:** use int64 for resolution search option ([5305efa](https://github.com/NateScarlet/pixiv/commit/5305efaa5f27082fe4f21bb561872be364b85477))


### Bug Fixes

* **artwork:** search by resolution not work ([0a5b7fe](https://github.com/NateScarlet/pixiv/commit/0a5b7fe89ab3695e0e85496e2cf353b10d51102f))
* **artwork:** search option WidthtGreaterThan -> WidthGreaterThan ([7a145b8](https://github.com/NateScarlet/pixiv/commit/7a145b8a51b52e0be741ffe3d8a7d9fc2e597f74))

### [0.4.2](https://github.com/NateScarlet/pixiv/compare/v0.4.1...v0.4.2) (2021-01-09)


### Features

* **artwork:** update the r18 mode in artwork search function ([#18](https://github.com/NateScarlet/pixiv/issues/18)) ([4079b08](https://github.com/NateScarlet/pixiv/commit/4079b08d50f2ff8951f01d22f3d59318b541dfa3))

### [0.4.1](https://github.com/NateScarlet/pixiv/compare/v0.4.0...v0.4.1) (2021-01-03)


### Features

* add novel.HTMLContent ([38d0c2b](https://github.com/NateScarlet/pixiv/commit/38d0c2b17e74e9091fa3ca0ff704fa6e9e1d76bc))
* full syntax support for novel.HTMLContent ([c24fbd5](https://github.com/NateScarlet/pixiv/commit/c24fbd572b3a8c4caa6c7648164d033f7f4d6fc5))


### Bug Fixes

* should set User-Agent header ([03a3b46](https://github.com/NateScarlet/pixiv/commit/03a3b46b3b0de7f5943631e792f71b0af766ab79)), closes [#17](https://github.com/NateScarlet/pixiv/issues/17)

## [0.4.0](https://github.com/NateScarlet/pixiv/compare/v0.3.5...v0.4.0) (2020-11-25)


### ⚠ BREAKING CHANGES

* add `pixiv: ` prefix to all errors
* add SearchOptions
* not prevent repeat fetch
* use client from context

### Features

* add `pixiv: ` prefix to all errors ([444af85](https://github.com/NateScarlet/pixiv/commit/444af85c41d1f8757871d393c2435cf1f856c71f))
* add SearchOptions ([4b085d8](https://github.com/NateScarlet/pixiv/commit/4b085d85842901033a0a4e60a9cad4abc86ba6a8))
* not prevent repeat fetch ([903ed9a](https://github.com/NateScarlet/pixiv/commit/903ed9a3fa96de25e8acf711e5deaec063dd52cd))
* use client from context ([ac7122c](https://github.com/NateScarlet/pixiv/commit/ac7122cdeb6c51c82a1d1a15d81744a1cb2fd033))

## [0.3.5](https://github.com/NateScarlet/pixiv/compare/v0.3.4...v0.3.5) (2020-10-15)

### Bug Fixes

- follow search api change ([bab1ecf](https://github.com/NateScarlet/pixiv/commit/bab1ecfadf459be0e7ed0f310e460e9e3fd3a6d0))

## [0.3.4](https://github.com/NateScarlet/pixiv/compare/v0.3.3...v0.3.4) (2020-09-15)

### Features

- add client.DNSQueryURL config ([8f53ffc](https://github.com/NateScarlet/pixiv/commit/8f53ffc05c26060124e3c6d2507ab984671e4c62))

### Bug Fixes

- bypass sni should check tls certs ([9bf1ac3](https://github.com/NateScarlet/pixiv/commit/9bf1ac38221b2774a488639576b274d5a8f7f3f4))
- RoundTripper should not modify request ([b52fa79](https://github.com/NateScarlet/pixiv/commit/b52fa794e75502d06514cca611804a54cd878a1b))
- tls cert verification should check expire time ([ad425c6](https://github.com/NateScarlet/pixiv/commit/ad425c628897908310983675fec513ec91f83085))

## [0.3.3](https://github.com/NateScarlet/pixiv/compare/v0.3.2...v0.3.3) (2020-09-15)

### Bug Fixes

- should use default transport when wrapped is nil ([e6fcd0c](https://github.com/NateScarlet/pixiv/commit/e6fcd0cd9455a4ba7c2bde168965c4ccede407c1))

## [0.3.2](https://github.com/NateScarlet/pixiv/compare/v0.3.1...v0.3.2) (2020-09-15)

### Features

- bypass sni blocking ([3793647](https://github.com/NateScarlet/pixiv/commit/3793647c0bd250b9ea5c3e1ab1e92880c48f7410)), closes [#8](https://github.com/NateScarlet/pixiv/issues/8) [#9](https://github.com/NateScarlet/pixiv/issues/9)

## [0.3.1](https://github.com/NateScarlet/pixiv/compare/v0.3.0...v0.3.1) (2020-04-06)

### Features

- add artwork.Rank.URL method ([ff172a4](https://github.com/NateScarlet/pixiv/commit/ff172a4c984ab19d62667645b53d75c6fdf014c5))

## [0.3.0](https://github.com/NateScarlet/pixiv/compare/v0.2.0...v0.3.0) (2020-04-06)

### ⚠ BREAKING CHANGES

- artwork.Page.URLs -> artwork.Page.Image ([dace691](https://github.com/NateScarlet/pixiv/commit/dace691cf717c56ef68309a9592c6c6a2ef7dec2))
- Artwork.URLs -> Artwork.Image ([5a57f6f](https://github.com/NateScarlet/pixiv/commit/5a57f6fad70096f3532d34658b344043b9f21765))
- client url functions to `URL` methods ([1033706](https://github.com/NateScarlet/pixiv/commit/1033706b032001904aad6fbefcac930f87219edc))
- User.AvatarURLs -> User.Avatar ([2d72011](https://github.com/NateScarlet/pixiv/commit/2d72011b2ef33e6f6d7f57d1cb4cfe91c94af764))

## [0.2.0](https://github.com/NateScarlet/pixiv/compare/v0.1.1...v0.2.0) (2020-04-04)

### ⚠ BREAKING CHANGES

- split packages ([1d95568](https://github.com/NateScarlet/pixiv/commit/1d955684115c6c59717617b3d7f2655f1e4bc73e))
