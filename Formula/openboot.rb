class Openboot < Formula
  desc "Set up your Mac dev environment in one command"
  homepage "https://openboot.dev"
  url "https://github.com/openbootdotdev/openboot/archive/refs/tags/v0.22.0.tar.gz"
  sha256 "6a4f6ae11d24963932599d46ca3f4c53067315da4003ab195aa51d6244b699d7"
  license "MIT"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X github.com/openbootdotdev/openboot/internal/cli.version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/openboot"
    generate_completions_from_executable(bin/"openboot", "completion")
  end

  test do
    assert_match "OpenBoot v#{version}", shell_output("#{bin}/openboot version")
    assert_match "Usage:", shell_output("#{bin}/openboot --help")
    assert_match "completion", shell_output("#{bin}/openboot --help")
  end
end
