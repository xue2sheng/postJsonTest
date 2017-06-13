# boost
if(EXISTS ${STATIC_BOOST})

  # static compilation for builtBoost libs
  set(Boost_NO_SYSTEM_PATHS TRUE)
  set(BOOST_ROOT "${STATIC_BOOST}")
  set(BOOST_INCLUDE_DIRS "${BOOST_ROOT}/include")
  set(BOOST_LIBRARY_DIRS "${BOOST_ROOT}/lib")
  set(Boost_USE_STATIC_LIBS ON)
  set(Boost_USE_MULTITHREADED ON)
  set(Boost_USE_STATIC_RUNTIME ON)

else()

  # dynamic compilation for local libs
  set(Boost_USE_STATIC_LIBS OFF)
  set(Boost_USE_MULTITHREADED ON)
  set(Boost_USE_STATIC_RUNTIME OFF)
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -DBOOST_TEST_DYN_LINK")

endif(EXISTS ${STATIC_BOOST})

message(STATUS "Using boost at ${BOOST_ROOT}")
find_package( Boost 1.58.0 REQUIRED COMPONENTS system filesystem)
if(Boost_FOUND)

 ## performace test doesn't belong to Boost test framework
 file(GLOB SOURCES *.cpp)
 include_directories(${CMAKE_CURRENT_SOURCE_DIR} ${Boost_INCLUDE_DIR})
 add_executable(${LOCAL_CMAKE_PROJECT_NAME} ${SOURCES})
 target_link_libraries(${LOCAL_CMAKE_PROJECT_NAME} ${Boost_SYSTEM_LIBRARY} ${Boost_FILESYSTEM_LIBRARY})

endif(Boost_FOUND)

