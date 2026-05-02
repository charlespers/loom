#include <loom/loom.h>
#include <gtest/gtest.h>
#include <cstdlib>

TEST(Smoke, InactiveByDefault) {
  unsetenv("LOOM_RUN_ID");
  EXPECT_EQ(loom::init(), 0);
  EXPECT_FALSE(loom::active());
  loom::shutdown();
}

TEST(Smoke, ActivatesWithEnvVar) {
  setenv("LOOM_RUN_ID", "01J3KTV6S5ABCDEFGHJKMNPQRS", 1);
  EXPECT_EQ(loom::init(), 0);
  EXPECT_TRUE(loom::active());
  loom::shutdown();
  EXPECT_FALSE(loom::active());
  unsetenv("LOOM_RUN_ID");
}

TEST(Smoke, CallsAreHarmlessWhenInactive) {
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
