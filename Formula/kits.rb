# Homebrew formula, served from this same repository as a tap:
#
#   brew tap alexvinola/kits https://github.com/alexvinola/kits
#   brew install alexvinola/kits/kits
#
# The version and checksums are pinned by scripts/bump-homebrew-formula.sh
# after each release; the all-zero hashes below are placeholders until the
# first one.
class Kits < Formula
  desc "Install instruction and skill kits into projects for any coding agent"
  homepage "https://github.com/alexvinola/kits"
  version "0.1.0"
  license "MIT"

  depends_on "git"

  on_macos do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-arm64"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-amd64"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-arm64"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-amd64"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
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
