#!/bin/sh

version=$(cat VERSION)
pwd

while IFS= read -r theme; do
    [ -z "$theme" ] && continue
    echo "Building theme: $theme"
    rm -rf "build/$theme"
    cd "$theme"
    npm install
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version npm run build
    cd ..
    mkdir -p build
    mv "$theme/build" "build/$theme"
done < THEMES
