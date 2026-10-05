# Homebrew formula, served from this same repository as a tap:
#
#   brew tap alexvinola/kits https://github.com/alexvinola/kits
#   brew install alexvinola/kits/kits
#
# The version and checksums are pinned by scripts/bump-homebrew-formula.sh
# after each release.
class Kits < Formula
  desc "Install instruction and skill kits into projects for any coding agent"
  homepage "https://github.com/alexvinola/kits"
  version "0.2.0"
  license "MIT"

  depends_on "git"

  on_macos do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-arm64"
      sha256 "b6a3db51e805b934b96c53bc5d2def10279af136a0e6e0b381e837f85229daa9"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-amd64"
      sha256 "798061eef3b3b9f3c235fe2aff4b3e8c4874c731b0a4a5f873ab5e0ccb89dcf1"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-arm64"
      sha256 "994b4ea2ed02676dc9ac96d3cc8da3915bb77c204f38abf45adcfdab1d8fe749"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-amd64"
      sha256 "5f94c719db0e592d8fd56e9f2da3c16cbe849f429e8aca93c7eca8ebbbeb2921"
    end
  end

  def install
    binary = Dir["kits-*"].first
    chmod "+x", binary
    bin.install binary => "kits"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kits version")
  end
end
