#!/bin/sh

version=$(cat VERSION)
pwd

while IFS= read -r theme; do
    [ -z "$theme" ] && continue
    echo "Building theme: $theme"
    cd "$theme"
    npm install
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version npm run build
    cd ..
    # package.json 中的 build 脚本已经将产物移至 ../build/<主题名>/
    # build.sh 中不再重复移动
done < THEMES
