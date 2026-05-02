cmake_minimum_required(VERSION 3.20)
project(loom
        VERSION 0.1.0
        DESCRIPTION "Observability harness for local-compute AI systems"
        LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Export compile_commands.json so clangd / IDEs can resolve include paths
# and the C++ standard. Placed at the build root; a symlink at the repo
# root keeps it discoverable without IDE configuration.
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

option(LOOM_BUILD_TESTS "Build embed-lib unit tests" ON)
option(LOOM_BUILD_EXAMPLES "Build example programs" ON)

if(LOOM_BUILD_TESTS)
  enable_testing()
endif()

add_subdirectory(embed)

if(LOOM_BUILD_EXAMPLES)
  add_subdirectory(examples/cpp_minimal)
endif()

# Symlink compile_commands.json into the repo root so clangd finds it
# immediately after configure. The file itself is generated during the
# Generate phase (after this CMakeLists.txt finishes processing); the
# symlink is created now and resolves once Generate writes the target.
if(CMAKE_EXPORT_COMPILE_COMMANDS AND NOT WIN32)
  file(CREATE_LINK
    ${CMAKE_BINARY_DIR}/compile_commands.json
    ${CMAKE_SOURCE_DIR}/compile_commands.json
    SYMBOLIC
  )
endif()
