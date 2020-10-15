# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

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
