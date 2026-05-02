#include <loom/loom.h>
#include <cstdio>

int main(int argc, char** argv) {
  (void)argc; (void)argv;
  loom::init();
  std::printf("loom active=%d\n", loom::active() ? 1 : 0);
  {
    loom::Span s("example.run", {{"step", 1}});
    loom::metric_f64("rate", 0.5);
    loom::lifecycle("done");
  }
  loom::shutdown();
  return 0;
}
