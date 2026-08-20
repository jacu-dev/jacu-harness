# frozen_string_literal: true
# Snapshot of the formula GoReleaser writes into the GitHub Release.
# The installable tap is jacu-dev/homebrew-jacu (brew install jacu-dev/jacu/jacu).

class Jacu < Formula
  desc "Governance harness for coding agents"
  homepage "https://github.com/jacu-dev/jacu-harness"
  version "0.2.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/jacu-dev/jacu-harness/releases/download/v0.2.0/jacu_0.2.0_darwin_amd64.tar.gz"
      sha256 "44e7f30782cd71f8a0c0a952415e7b5bcf860d906ead1fa7701b77706f4d8d31"
    end
    on_arm do
      url "https://github.com/jacu-dev/jacu-harness/releases/download/v0.2.0/jacu_0.2.0_darwin_arm64.tar.gz"
      sha256 "95927af51dcf7227651cd75e1fd51d912adcdeac19a994a4731c25ff0ef7d467"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/jacu-dev/jacu-harness/releases/download/v0.2.0/jacu_0.2.0_linux_amd64.tar.gz"
      sha256 "28ea495b2f23e992b424cadf81c581ea3d9f8201068f901a0c06aff682747dca"
    end
    on_arm do
      url "https://github.com/jacu-dev/jacu-harness/releases/download/v0.2.0/jacu_0.2.0_linux_arm64.tar.gz"
      sha256 "6af32ed373436ca6799ba506c045ecd4a9dd20b1f2d076f2cc6b42810e72c3bc"
    end
  end

  def install
    bin.install "jacu"
    bin.install_symlink "jacu" => "jacu-mcp"
  end

  test do
    assert_match "0.2.0", shell_output("#{bin}/jacu version")
  end
end
