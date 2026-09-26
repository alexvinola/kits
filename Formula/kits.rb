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
  version "0.1.0"
  license "MIT"

  depends_on "git"

  on_macos do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-arm64"
      sha256 "c6e82e95b08dd729b4b0a7aa008a148e35c17204f2ac9e6d432b109b99dc8e84"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-darwin-amd64"
      sha256 "ac887f63eec95ca4693cbb9031621ccbd3e1042b1e5487856475d34bcafe8ede"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-arm64"
      sha256 "8c9925df5cf2d658266cf91c60155adf179639e19e4f8d564dc5dfddf03517a9"
    end
    on_intel do
      url "https://github.com/alexvinola/kits/releases/download/v#{version}/kits-linux-amd64"
      sha256 "aa72a4b70cff5c0889c6b98a87909265b9a47bebb42b87c5acf3f1bc7dcb7f3e"
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
