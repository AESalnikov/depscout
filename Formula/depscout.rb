# Local/dev formula (install from a clone of this repo).
# Preferred install after a release publishes the tap cask:
#   brew install --cask AESalnikov/tap/depscout
#
# From this repo (sha256 must match a published GitHub Release):
#   brew install --formula ./Formula/depscout.rb
class Depscout < Formula
  desc "Scout and update dependency versions in Gradle and Maven projects"
  homepage "https://github.com/AESalnikov/depscout"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/AESalnikov/depscout/releases/download/v#{version}/depscout_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_AFTER_RELEASE"
    end
    on_intel do
      url "https://github.com/AESalnikov/depscout/releases/download/v#{version}/depscout_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_AFTER_RELEASE"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/AESalnikov/depscout/releases/download/v#{version}/depscout_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_AFTER_RELEASE"
    end
    on_intel do
      url "https://github.com/AESalnikov/depscout/releases/download/v#{version}/depscout_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_AFTER_RELEASE"
    end
  end

  def install
    bin.install "depscout"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/depscout --version")
  end
end
