include(FetchContent)
FetchContent_Declare(
  googletest
  GIT_REPOSITORY https://github.com/google/googletest.git
  GIT_TAG        v1.14.0
)
set(BUILD_GMOCK OFF CACHE BOOL "" FORCE)
set(INSTALL_GTEST OFF CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

add_executable(loom_test_smoke test_smoke.cpp)
target_link_libraries(loom_test_smoke
  PRIVATE loom GTest::gtest_main
)

include(GoogleTest)
gtest_discover_tests(loom_test_smoke)
