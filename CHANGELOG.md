# Changelog

## [0.3.1](https://github.com/highperformance-tech/ana-cli/compare/v0.3.0...v0.3.1) (2026-04-23)


### Features

* **api:** ana api raw-JSON passthrough verb ([#25](https://github.com/highperformance-tech/ana-cli/issues/25)) ([#26](https://github.com/highperformance-tech/ana-cli/issues/26)) ([b16cadf](https://github.com/highperformance-tech/ana-cli/commit/b16cadfc89925209bbe9f0549302978d5ad3345c))

## [0.3.0](https://github.com/highperformance-tech/ana-cli/compare/v0.2.1...v0.3.0) (2026-04-22)


### ⚠ BREAKING CHANGES

* **connector:** dialect and auth-mode as subcommands (postgres) ([#15](https://github.com/highperformance-tech/ana-cli/issues/15))

### Features

* **cli:** groups can declare inheritable flags ([#11](https://github.com/highperformance-tech/ana-cli/issues/11)) ([bc374aa](https://github.com/highperformance-tech/ana-cli/commit/bc374aade1c849b824a988a1c9744f526ccfef45))
* **connector:** Databricks create — 4 auth modes with unit + e2e coverage ([#21](https://github.com/highperformance-tech/ana-cli/issues/21)) ([ba9159d](https://github.com/highperformance-tech/ana-cli/commit/ba9159d741b5649f770b688e0ce9dda733e54d53))
* **connector:** Snowflake create — password, key-pair, OAuth SSO, OAuth individual ([#17](https://github.com/highperformance-tech/ana-cli/issues/17)) ([c0f6494](https://github.com/highperformance-tech/ana-cli/commit/c0f6494d43200b308786a215d16ef2b508ed61b8))
* **e2e:** comprehensive live-smoke coverage across all captured verbs ([#18](https://github.com/highperformance-tech/ana-cli/issues/18)) ([f739b38](https://github.com/highperformance-tech/ana-cli/commit/f739b3846df4b6a636cbb6c0190e1ab89cc94817))
* **update:** update-check nudge + `ana update` self-update verb ([#23](https://github.com/highperformance-tech/ana-cli/issues/23)) ([8da2017](https://github.com/highperformance-tech/ana-cli/commit/8da2017aaa61a870c4501274f9b2156df44370e5))


### Bug Fixes

* **cli:** global flags visible, position-tolerant, and surface usage errors ([#10](https://github.com/highperformance-tech/ana-cli/issues/10)) ([33bb9c3](https://github.com/highperformance-tech/ana-cli/commit/33bb9c325be53b927d35619aaff561ae951ff2d1))
* **connector:** test verb fetches config via GetConnector before probing ([#19](https://github.com/highperformance-tech/ana-cli/issues/19)) ([e9f3616](https://github.com/highperformance-tech/ana-cli/commit/e9f36160f6f380585654bfdbef918ad33d3b5654))


### Code Refactoring

* **connector:** dialect and auth-mode as subcommands (postgres) ([#15](https://github.com/highperformance-tech/ana-cli/issues/15)) ([6edf0bd](https://github.com/highperformance-tech/ana-cli/commit/6edf0bdd1571df753b1130f9236e5798174580e7))

## [0.2.1](https://github.com/highperformance-tech/ana-cli/compare/v0.2.0...v0.2.1) (2026-04-20)


### Bug Fixes

* **cli:** render leaf Help on --help/-h ([#18](https://github.com/highperformance-tech/ana-cli/issues/18)) ([de9e7d6](https://github.com/highperformance-tech/ana-cli/commit/de9e7d6fa88d9bc78a474a80e061f3d447bf3552))

## [0.2.0](https://github.com/highperformance-tech/ana-cli/compare/v0.1.0...v0.2.0) (2026-04-19)


### Features

* **audit:** add tail command with --since/--limit flags ([7d573e2](https://github.com/highperformance-tech/ana-cli/commit/7d573e2e8a8ece2c024494853e3e46e748d710c8))
* **auth:** add login/logout/whoami and keys + service-accounts groups ([82c43ee](https://github.com/highperformance-tech/ana-cli/commit/82c43eeeb6b293277b0bcf4234998dd3f23b4921))
* **auth:** whoami shows active org ([bbbefea](https://github.com/highperformance-tech/ana-cli/commit/bbbefeab75382b8c9f2e60d5d3f3989304f6f7fc))
* **chat:** scaffold chat verb package with unary commands and streaming send ([f367f73](https://github.com/highperformance-tech/ana-cli/commit/f367f73d29e83792527989d0e1c21e496d39e6e4))
* **cli:** add argument dispatch core ([797ca6a](https://github.com/highperformance-tech/ana-cli/commit/797ca6a8e4f5b2fe5df2eabab962d5ec0463440d))
* **cmd:** wire ana CLI main entrypoint ([f4e4d13](https://github.com/highperformance-tech/ana-cli/commit/f4e4d1360357208a4d4516ad112129f0dd2d0832))
* **config:** add JSON config load/save with env overrides ([d36b8b8](https://github.com/highperformance-tech/ana-cli/commit/d36b8b88de86a353df64025d5bd6bcb5fe8d6294))
* **connector:** add list/get/create/update/delete/test/tables/examples commands ([a2d253f](https://github.com/highperformance-tech/ana-cli/commit/a2d253f26ad067b94ac8136472edfb7cc6a9edb1))
* **dashboard:** add dashboard verb tree (list/folders/get/spawn/health) ([dc440d3](https://github.com/highperformance-tech/ana-cli/commit/dc440d3ca2fe63e7feb8aa5c46be35a0a6106b3f))
* **e2e:** add live smoke test harness against app.textql.com ([6ac0811](https://github.com/highperformance-tech/ana-cli/commit/6ac0811859853409932358aee979bd5c4ee665b4))
* **e2e:** source .env in e2e make targets and skip dryrun assertions ([58f01c8](https://github.com/highperformance-tech/ana-cli/commit/58f01c81e3a1a596e117983068f7a1756a2b8c4e))
* **feed:** add show and stats commands ([26982e4](https://github.com/highperformance-tech/ana-cli/commit/26982e4ac9a10e05b9ec13ac59381ccf0b0532cc))
* **ontology:** add list and get commands ([340e71d](https://github.com/highperformance-tech/ana-cli/commit/340e71de3ec1042d65d02ff72fdc973ff2c685d7))
* **org:** add list subcommand ([93290b6](https://github.com/highperformance-tech/ana-cli/commit/93290b626891436cde0d81d717be30796a9258fd))
* **org:** add org package with show/members/roles/permissions commands ([d77de93](https://github.com/highperformance-tech/ana-cli/commit/d77de9364e47f5fe6259d73e35443eb972b252a7))
* **playbook:** add list/get/reports/lineage verbs ([36c6722](https://github.com/highperformance-tech/ana-cli/commit/36c6722ec28df70f91f35bffa81c6692430c2a0f))
* **profile:** add ana profile verb ([cba89e1](https://github.com/highperformance-tech/ana-cli/commit/cba89e1eeae3594d1643a700a1123983c811996d))
* publish release pipeline + install.sh + version banner ([5fbf9dd](https://github.com/highperformance-tech/ana-cli/commit/5fbf9dde264eced91864401f607e23f643a4087a))
* **transport:** Connect-RPC JSON client with unary + streaming ([96c142d](https://github.com/highperformance-tech/ana-cli/commit/96c142d3598f42aa73657da12db1a43f553c5488))


### Bug Fixes

* **chat:** support flags interspersed with positional args ([3f45045](https://github.com/highperformance-tech/ana-cli/commit/3f450458f607cb625e05fe0cad5b6ac04fc29b51))
* **cli:** centralize flag parsing to keep trailing flags after positionals ([338bd52](https://github.com/highperformance-tech/ana-cli/commit/338bd521cbbfd28f9b3f8e15fdff16490e65cfa2))
* **cli:** return exit 0 for --help ([80cc3ba](https://github.com/highperformance-tech/ana-cli/commit/80cc3ba46b737b07c62013bceac64dfb269aa287))
* **cli:** treat whitespace-only int IDs as missing argument ([#14](https://github.com/highperformance-tech/ana-cli/issues/14)) ([454fdec](https://github.com/highperformance-tech/ana-cli/commit/454fdecaaefc8600173f183c359b3015a96b1ba6))
* **connector:** pre-fetch baseline and honor interleaved flags on update ([1433e01](https://github.com/highperformance-tech/ana-cli/commit/1433e012c5c83f5cda426a4e1f563d9f87727e2a))
* **e2e,transport:** connect streaming framing and rbac create shapes ([c07d528](https://github.com/highperformance-tech/ana-cli/commit/c07d528d6244a4b64af464badec721c3f9c368af))
* **org:** send orgId in ListOrganizationMembers request ([233eb22](https://github.com/highperformance-tech/ana-cli/commit/233eb222ec8f2043831e71f83cd79853ea1809fa))
