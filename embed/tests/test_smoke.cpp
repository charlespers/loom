#include <loom/loom.h>
#include <gtest/gtest.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <sstream>
#include <string>

namespace {

// Per-test sandbox: redirect $LOOM_HOME so events.jsonl never touches the
// developer's real ~/.loom.
class LoomEnv {
 public:
  LoomEnv() {
    char tmpl[] = "/tmp/loom_test_XXXXXX";
    char* dir = mkdtemp(tmpl);
    if (dir) home_ = dir;
    setenv("LOOM_HOME", home_.c_str(), 1);
  }
  ~LoomEnv() {
    unsetenv("LOOM_HOME");
    unsetenv("LOOM_RUN_ID");
    // Tempdir cleanup deferred to OS / tmpwatch; tests are short-lived.
  }
  const std::string& home() const { return home_; }
 private:
  std::string home_;
};

std::string slurp(const std::string& path) {
  std::ifstream is(path);
  std::ostringstream buf;
  buf << is.rdbuf();
  return buf.str();
}

}  // namespace

TEST(Smoke, InactiveByDefault) {
  LoomEnv env;
  unsetenv("LOOM_RUN_ID");
  EXPECT_EQ(loom::init(), 0);
  EXPECT_FALSE(loom::active());
  loom::shutdown();
}

TEST(Smoke, ActivatesWithEnvVar) {
  LoomEnv env;
  setenv("LOOM_RUN_ID", "01J3KTV6S5ABCDEFGHJKMNPQRS", 1);
  EXPECT_EQ(loom::init(), 0);
  EXPECT_TRUE(loom::active());
  loom::shutdown();
  EXPECT_FALSE(loom::active());
}

TEST(Smoke, CallsAreHarmlessWhenInactive) {
  LoomEnv env;
  unsetenv("LOOM_RUN_ID");
  loom::init();
  ASSERT_FALSE(loom::active());
  // None of these may crash; all are no-ops.
  { loom::Span s("probe", {{"k", 1}}); }
  loom::metric_f64("rate", 0.5);
  loom::counter_inc("hits");
  loom::error("oops", "boom", loom::Severity::Warn);
  loom::audit("file.read", {{"path", std::string_view("/x")}});
  loom::lifecycle("run.start");
  loom::shutdown();
  SUCCEED();
}

TEST(EventsWriter, EmitsNDJSONWhenActive) {
  LoomEnv env;
  const char kRunID[] = "01J3KTV6S5EVENTSWRITERTEST";
  setenv("LOOM_RUN_ID", kRunID, 1);
  ASSERT_EQ(loom::init(), 0);
  ASSERT_TRUE(loom::active());

  { loom::Span s("forward.layer", {{"layer", 8}}); }
  loom::metric_f64("tok_per_s", 111.5);
  loom::audit("file.read", {{"path", std::string_view("/x")}});
  loom::lifecycle("run.start");
  loom::error("cuda.fail", "kernel launch failed", loom::Severity::Error);
  loom::shutdown();

  std::string path = env.home() + "/runs/" + kRunID + "/events.jsonl";
  std::string contents = slurp(path);
  ASSERT_FALSE(contents.empty()) << "events.jsonl was not produced at " << path;

  // One line per call. Lines we emitted: span(forward.layer), metric,
  // audit, lifecycle, error → 5.
  size_t lines = 0;
  for (char c : contents) if (c == '\n') ++lines;
  EXPECT_EQ(lines, 5u);

  // Each line is well-formed enough that we can find the schema tag and category.
  EXPECT_NE(contents.find("\"v\":\"loom.event.v1\""), std::string::npos);
  EXPECT_NE(contents.find("\"cat\":\"span\""),         std::string::npos);
  EXPECT_NE(contents.find("\"name\":\"forward.layer\""), std::string::npos);
  EXPECT_NE(contents.find("\"cat\":\"metric\""),       std::string::npos);
  EXPECT_NE(contents.find("\"cat\":\"audit\""),        std::string::npos);
  EXPECT_NE(contents.find("\"cat\":\"lifecycle\""),    std::string::npos);
  EXPECT_NE(contents.find("\"cat\":\"error\""),        std::string::npos);
}
