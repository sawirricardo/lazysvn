class Lazysvn < Formula
  desc "LazyGit-style terminal UI for Subversion (SVN)"
  homepage "https://github.com/sawirricardo/lazysvn"
  head "https://github.com/sawirricardo/lazysvn.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "."
  end

  test do
    assert_match "LazySVN", shell_output("#{bin}/lazysvn --help")
  end
end
