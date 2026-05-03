// examples/cpp_minimal/main.cpp — short tour of the embed library.
//
// Demonstrates every event category with realistic shapes so the
// resulting events.jsonl + summary.md show what a useful run looks like.
// Compiled as `cpp_minimal` and exercised by tools/smoke.sh and the
// loom_demo.sh script.
#include <loom/loom.h>

#include <chrono>
#include <cstdio>
#include <thread>

int main() {
  loom::init();
  std::printf("loom active=%d\n", loom::active() ? 1 : 0);

  loom::lifecycle("setup", {{"phase", "weights"}});

  // Audit the input the model will see — the kind of thing a compliance
  // operator wants pinned in audit.jsonl. Sync mode by default.
  //
  // Field-level disclosure: keys ending in "@public" are kept in
  // audit.public.jsonl (with the suffix stripped); other keys stay
  // private. So an external auditor sees the file size + a hash of
  // the path, but not the path itself. This is the spec § 4.3
  // redaction convention; nothing else needs to be configured.
  loom::audit("file.read", {
      {"path",             std::string_view("/Users/example/notes.md")},
      {"bytes@public",     int64_t{12384}},
      {"path_hash@public", std::string_view("sha256:9f2c…")},
  });

  loom::lifecycle("decode.start");

  // Simulate a 12-layer forward pass twice (two tokens). Each layer is a
  // span with attributes; tok_step_ms is a metric per token; nested spans
  // get parent IDs through the per-thread stack automatically.
  for (int step = 0; step < 2; step++) {
    loom::Span tok("forward.step", {{"step", int64_t(step)}});
    for (int layer = 0; layer < 12; layer++) {
      loom::Span lay("forward.layer", {
          {"layer",   int64_t(layer)},
          {"variant", layer % 4 == 0 ? "qkv_tri" : "baseline"},
      });
      // Pretend to do work.
      std::this_thread::sleep_for(std::chrono::microseconds(120));
      loom::counter_inc("kernels_launched", 5);
    }
    loom::metric_f64("tok_step_ms", 8.71 + 0.05 * step,
                     {{"step", int64_t(step)}});
  }

  loom::lifecycle("decode.end");

  // A non-fatal warning to show the error category populates summary.md.
  loom::error("cuda.lazy_flush_warning",
              "stream submission stalled (CUDA 11.4 quirk; recovered)",
              loom::Severity::Warn,
              {{"step", int64_t{1}}});

  // Final audit closing the run with an output hash. Both fields are
  // marked @public — the external auditor wants to see deterministic
  // output identity and how many tokens we generated.
  loom::audit("run.finished", {
      {"output_hash@public", std::string_view("a1b2c3d4")},
      {"tokens@public",      int64_t{2}},
  });

  loom::shutdown();
  std::puts("cpp_minimal: done — see $LOOM_HOME/runs/<id>/{summary.md,events.jsonl}");
  return 0;
}
