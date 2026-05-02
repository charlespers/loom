cmake_minimum_required(VERSION 3.20)
project(loom
        VERSION 0.1.0
        DESCRIPTION "Observability harness for local-compute AI systems"
        LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

option(LOOM_BUILD_TESTS "Build embed-lib unit tests" ON)
option(LOOM_BUILD_EXAMPLES "Build example programs" ON)

add_subdirectory(embed)

if(LOOM_BUILD_EXAMPLES)
  add_subdirectory(examples/cpp_minimal)
endif()
