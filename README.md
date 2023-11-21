# grank

This repository contains the source code of the tools used by [grank.io](http://grank.dev).

To be able to compute the PageRank of the golang github repositories the following steps are needed:

1. Fetching the list of golang github repositories: `lsrepo` quesries the github api.
2. Downloading the source code from the repositories: `fetcharchive` downloads the archive of the repositories collected by `lsrepo` and extracts them without their stored dependencies (vendor folder).
3. Building up the module dependency graph and computing the repo starcount weighted pagrank `modranker`

To performe these steps one can run the following commands in the root of the repository:
NOTE: a github api token is required. 

```
REPOS_JSON="repos.json"
GH_TOKEN=GITHUB_API_TOKEN go run ./lsrepo/ ${REPOS_JSON} 2> ${REPOS_JSON}.log

DOWNLOAD_DIR=`pwd`"/repos/"
go run ./fetcharchive/ -rep ${REPOS_JSON} -d ${DOWNLOAD_DIR} 2> fetch_arch.log

DG="dg.json"
go run ./modranker/ -r ${REPOS_JSON} -o ${DG} -d repos/ > wrank.csv 2> wrank.log

```
