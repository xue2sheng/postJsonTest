####################################
# build/install/pack instructions  #
####################################

### This can be placed in a specif CMake config file for each machine and use "find_package" ###
if(APPLE)
   set(CMAKE_C_COMPILER "/usr/bin/clang")
   set(CMAKE_CXX_COMPILER "/usr/bin/clang++")
   project(${LOCAL_CMAKE_PROJECT_NAME} CXX)
   set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -std=c++1z -g")
   set(CMAKE_LINKER_FLAGS "${CMAKE_LINKER_FLAGS} -std=c++1z -g")
elseif(UNIX)
  find_program(LSB_RELEASE lsb_release)
  execute_process(COMMAND ${LSB_RELEASE} -is OUTPUT_VARIABLE LSB_RELEASE_ID_SHORT OUTPUT_STRIP_TRAILING_WHITESPACE)
  if("${LSB_RELEASE_ID_SHORT}" MATCHES "Debian") # Supposed be at home so default g++ is already version 6
   #set(STATIC_BOOST "${COMPILATIONDEP}/builtBoost/debianGCC6")
   #set(SPECIAL_AR "gcc-ar")
   project(${LOCAL_CMAKE_PROJECT_NAME} CXX)
  else() # Supposed be at work, opensuse, where default g++ is old 4.x
   set(CMAKE_C_COMPILER "/usr/bin/gcc-6")
   set(CMAKE_CXX_COMPILER "/usr/bin/g++-6")
   #set(SPECIAL_AR "gcc-ar-6")
   #set(STATIC_BOOST "${COMPILATIONDEP}/builtBoost/openSuseGCC6")
   project(${LOCAL_CMAKE_PROJECT_NAME} CXX)
  endif("${LSB_RELEASE_ID_SHORT}" MATCHES "Debian")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -pthread -fPIC -std=gnu++1z -g")
  set(CMAKE_LINKER_FLAGS "${CMAKE_LINKER_FLAGS} -pthread -fPIC -std=gnu++1z -g")
endif(APPLE)
