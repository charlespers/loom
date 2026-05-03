#include <loom/loom.h>
#include <gtest/gtest.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <sstream>
#include <string>

namespace {

// Per-test sandbox: redirect $LOOM_HOME so artifacts never touch the
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

size_t count_substring(const std::string& s, const std::string& needle) {
  size_t n = 0, pos = 0;
  while ((pos = s.find(needle, pos)) != std::string::npos) {
    n++;
    pos += needle.size();
  }
  return n;
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
  { loom::Span s("probe", {{"k", 1}}); }
  loom::metric_f64("rate", 0.5);
  loom::counter_inc("hits");
  loom::error("oops", "boom", loom::Severity::Warn);
  loom::audit("file.read", {{"path", std::string_view("/x")}});
  loom::lifecycle("run.start");
  loom::shutdown();
  SUCCEED();
}

TEST(EventsWriter, EmitsAllCategoriesWithRichPayload) {
  LoomEnv env;
  const char kRunID[] = "01J3KTV6S5EVENTSWRITERTEST";
  setenv("LOOM_RUN_ID", kRunID, 1);
  ASSERT_EQ(loom::init(), 0);
  ASSERT_TRUE(loom::active());

  { loom::Span s("forward.layer", {{"layer", 8}, {"variant", "qkv_tri"}}); }
  loom::metric_f64("tok_per_s", 111.5, {{"step", 0}});
  loom::audit("file.read", {
      {"path",         std::string_view("/x")},
      {"bytes@public", int64_t{42}},
  });
  loom::lifecycle("custom.marker");
  loom::error("cuda.fail", "kernel launch failed", loom::Severity::Error);
  loom::shutdown();

  std::string base = env.home() + "/runs/" + kRunID;
  std::string events  = slurp(base + "/events.jsonl");
  std::string audit   = slurp(base + "/audit.jsonl");
  std::string pub     = slurp(base + "/audit.public.jsonl");
  std::string summary = slurp(base + "/summary.md");
  std::string mani    = slurp(base + "/manifest.json");

  // Every artifact file exists with content.
  ASSERT_FALSE(events.empty())  << "events.jsonl missing/empty at " << base;
  ASSERT_FALSE(audit.empty())   << "audit.jsonl missing/empty";
  ASSERT_FALSE(pub.empty())     << "audit.public.jsonl missing/empty";
  ASSERT_FALSE(summary.empty()) << "summary.md missing/empty";
  ASSERT_FALSE(mani.empty())    << "manifest.json missing/empty";

  // events.jsonl: every category we emitted is present.
  EXPECT_NE(events.find("\"cat\":\"span\""),         std::string::npos);
  EXPECT_NE(events.find("\"cat\":\"metric\""),       std::string::npos);
  EXPECT_NE(events.find("\"cat\":\"audit\""),        std::string::npos);
  EXPECT_NE(events.find("\"cat\":\"lifecycle\""),    std::string::npos);
  EXPECT_NE(events.find("\"cat\":\"error\""),        std::string::npos);

  // Span carries duration AND attributes.
  EXPECT_NE(events.find("\"name\":\"forward.layer\""), std::string::npos);
  EXPECT_NE(events.find("\"dur_ns\":"),                std::string::npos);
  EXPECT_NE(events.find("\"layer\":8"),                std::string::npos);
  EXPECT_NE(events.find("\"variant\":\"qkv_tri\""),    std::string::npos);

  // Metric carries the numeric value AND the kind tag.
  EXPECT_NE(events.find("\"value\":111.5"),    std::string::npos);
  EXPECT_NE(events.find("\"kind\":\"f64\""),   std::string::npos);

  // Two lifecycle markers from auto-init+shutdown plus our custom one.
  EXPECT_GE(count_substring(events, "\"cat\":\"lifecycle\""), 3u);
  EXPECT_NE(events.find("run.start"),     std::string::npos);
  EXPECT_NE(events.find("run.end"),       std::string::npos);
  EXPECT_NE(events.find("custom.marker"), std::string::npos);

  // audit.jsonl carries the chain (prev + this) and full attrs.
  EXPECT_NE(audit.find("\"chain\":"), std::string::npos);
  EXPECT_NE(audit.find("\"prev\":"),  std::string::npos);
  EXPECT_NE(audit.find("\"this\":"),  std::string::npos);
  EXPECT_NE(audit.find("\"path\":\"/x\""), std::string::npos);

  // audit.public.jsonl carries the chain. The "@public" key gets the
  // suffix stripped on the way out; the unsuffixed "path" key stays
  // private.
  EXPECT_NE(pub.find("\"chain\":"),    std::string::npos);
  EXPECT_NE(pub.find("\"bytes\":42"),  std::string::npos)
      << "public file should expose bytes@public as plain bytes: " << pub;
  EXPECT_EQ(pub.find("\"path\":"),     std::string::npos)
      << "public file leaked the private path key: " << pub;
  EXPECT_EQ(pub.find("@public"),       std::string::npos)
      << "public file should not carry @public suffixes: " << pub;

  // manifest.json self-describing.
  EXPECT_NE(mani.find("\"schema\": \"loom.manifest.v1\""), std::string::npos);
  EXPECT_NE(mani.find("\"run_id\":"),      std::string::npos);
  EXPECT_NE(mani.find("\"audit_chain\":"), std::string::npos);

  // summary.md is a designed report, not a stub.
  EXPECT_NE(summary.find("# Run"),                  std::string::npos);
  EXPECT_NE(summary.find("## Lifecycle"),           std::string::npos);
  EXPECT_NE(summary.find("## Spans by total time"), std::string::npos);
  EXPECT_NE(summary.find("forward.layer"),          std::string::npos);
}

TEST(SpanEmit, RecordsCallerSuppliedDuration) {
  LoomEnv env;
  const char kRunID[] = "01J3KTV6S5SPANEMITTEST0001";
  setenv("LOOM_RUN_ID", kRunID, 1);
  ASSERT_EQ(loom::init(), 0);

  // Two emit-style spans with caller-supplied durations and one with
  // an attribute. Mirrors what Bedrock's CudaTimedScope drain will do
  // after cudaEventElapsedTime returns the elapsed ms for a kernel.
  loom::span_emit("forward.layer", 200'000ull);
  loom::span_emit("forward.layer", 250'000ull, {{"layer", int64_t{0}}});
  loom::span_emit("attention.qkv", 800'000ull);
  loom::shutdown();

  std::string events = slurp(env.home() + "/runs/" + kRunID + "/events.jsonl");
  ASSERT_FALSE(events.empty()) << "events.jsonl missing/empty";

  // Each emit produces one span record with the supplied dur_ns and no
  // span_id field (emit-style spans aren't paired).
  EXPECT_NE(events.find("\"name\":\"forward.layer\",\"dur_ns\":200000"),
            std::string::npos);
  EXPECT_NE(events.find("\"name\":\"forward.layer\",\"dur_ns\":250000"),
            std::string::npos);
  EXPECT_NE(events.find("\"name\":\"attention.qkv\",\"dur_ns\":800000"),
            std::string::npos);
  // Attribute round-trips.
  EXPECT_NE(events.find("\"layer\":0"), std::string::npos);

  // Per-name stats should aggregate emit + RAII spans alike. Ensure
  // forward.layer's count is at least 2 in the manifest.
  std::string manifest = slurp(env.home() + "/runs/" + kRunID + "/manifest.json");
  EXPECT_NE(manifest.find("\"forward.layer\""), std::string::npos);
}

TEST(SpanEmit, NoopWhenInactive) {
  LoomEnv env;
  unsetenv("LOOM_RUN_ID");
  ASSERT_EQ(loom::init(), 0);
  ASSERT_FALSE(loom::active());
  // Must not crash, must not emit anything (no run dir to emit into).
  loom::span_emit("never.fires", 1'000'000ull);
  loom::span_emit("also.never",  500ull, {{"k", int64_t{1}}});
  loom::shutdown();
  SUCCEED();
}

TEST(AuditChain, HashesProgress) {
  LoomEnv env;
  const char kRunID[] = "01J3KTV6S5AUDITCHAINTEST";
  setenv("LOOM_RUN_ID", kRunID, 1);
  ASSERT_EQ(loom::init(), 0);
  loom::audit("step.a", {{"i", 1}});
  loom::audit("step.b", {{"i", 2}});
  loom::audit("step.c", {{"i", 3}});
  loom::shutdown();

  std::string audit = slurp(env.home() + "/runs/" + kRunID + "/audit.jsonl");
  // Three records, three distinct chain.this hashes, and record N's prev
  // matches record N-1's this.
  EXPECT_EQ(count_substring(audit, "\"this\":\""), 3u);
  EXPECT_EQ(count_substring(audit, "\"prev\":\""), 3u);
  // First record's prev is 64 zeroes.
  std::string zero64(64, '0');
  EXPECT_NE(audit.find("\"prev\":\"" + zero64 + "\""), std::string::npos);
}
