if(LOCAL_CMAKE_PROJECT_NAME)

  ### Code & Test ###
  add_subdirectory("src")

else()

  ### As an independent project ###
  cmake_minimum_required(VERSION 3.0)
  set(LOCAL_CMAKE_PROJECT_NAME JsonMock) 
  project(${LOCAL_CMAKE_PROJECT_NAME} NONE) 

  ### pseudo install ###
  set(BINARY_INSTALL_DIR ${CMAKE_CURRENT_BINARY_DIR})

  ### Code & Test ###
  add_subdirectory("src")

endif(LOCAL_CMAKE_PROJECT_NAME)

