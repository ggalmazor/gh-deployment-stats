#!/usr/bin/env bash

rm -rf build
rm -rf dist

npm install
npm run build

declare -A platform_arch_map
platform_arch_map["macos-x64"]="darwin-amd64"
platform_arch_map["macos-arm64"]="darwin-arm64"
platform_arch_map["linux-x64"]="linux-amd64"
platform_arch_map["linux-arm64"]="linux-arm64"
platform_arch_map["win-x64"]="windows-amd64"
platform_arch_map["win-arm64"]="windows-arm64"

input_file="build/index.js"
output_dir="dist"
node_version="node18"

mkdir -p "$output_dir"

for pkg_combination in "${!platform_arch_map[@]}"; do
  output_combination=${platform_arch_map[$pkg_combination]}

  platform_name=$(echo "$output_combination" | cut -d'-' -f1)
  arch_name=$(echo "$output_combination" | cut -d'-' -f2)

  output_file="$output_dir/gh-deployment-stats-$platform_name-$arch_name"

  if [ "$platform_name" == "windows" ]; then
    output_file="$output_file.exe"
  fi

  echo "Building for $pkg_combination (output: $output_combination)..."
  npx pkg "$input_file" --targets "$node_version-$pkg_combination" --output "$output_file"

  if [ $? -eq 0 ]; then
    echo "Successfully built $output_file"
  else
    echo "Failed to build for $pkg_combination"
  fi

  echo "-----------------------------------"
done