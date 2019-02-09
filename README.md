# grank

This repository contains the source code of the tools used by [grank.io](http://grank.io).

To be able to compute the PageRank of the golang github repositories the following steps are needed:

1. Fetching the list of golang github repositories: `lsrepo` quesries the github api.
2. Downloading the source code from the repositories: `fetcharchive` downloads the archive of the repositories collected by `lsrepo` and extracts them without their stored dependencies (vendor folder).
3. Colecting the imported packages for each source files to list the dependecies: `buildg`
4. Finding the github repositories for the imported packages in case when it is not obvious (like k8s.io -> github.com/kubernetes): `resolver`
5. Combine the collected data to compute the weighted pagerank: `wranker`

To performe these steps one can run the following commands in the root of the repository:
NOTE: the repository should be placed into the GOPATH and a github api token is required. 

```
REPOS_JSON="repos.json"
GH_TOKEN=GITHUB_API_TOKEN go run ./lsrepo/ ${REPOS_JSON} 2> ${REPOS_JSON}.log

DOWNLOAD_DIR=`pwd`"/repos/"
go run ./fetcharchive/ -rep ${REPOS_JSON} -d ${DOWNLOAD_DIR} 2> fetch_arch.log

DEPS="deps.csv"
go run ./buildg/ -r ${DOWNLOAD_DIR}github.com -o ${DEPS} 2> ${DEPS}.log

PKG_REPOS="pkg_repos.json"
if [ ! -f ${PKG_REPOS} ]; then
    go run ./resolver/ -d ${DEPS} -o ${PKG_REPOS}  2> resolver.log
else
    go run ./resolver/ -d ${DEPS} -o ${PKG_REPOS} -r ${PKG_REPOS} 2> resolver.log
fi

DG="dg.json"
go run ./wranker/ -r ${REPOS_JSON} -d ${DEPS} -ru ${PKG_REPOS} -o ${DG} -pref "grank/repos/" -s -v > wrank.csv 2> wrank.log

```
