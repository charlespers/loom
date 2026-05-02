add_library(loom STATIC
  src/loom.cpp
)

target_include_directories(loom
  PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)

target_compile_options(loom PRIVATE
  -Wall -Wextra -Wpedantic -Wshadow -Wconversion
)

set_target_properties(loom PROPERTIES
  OUTPUT_NAME "loom"
)

if(LOOM_BUILD_TESTS)
  if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/tests/CMakeLists.txt)
    enable_testing()
    add_subdirectory(tests)
  endif()
endif()
