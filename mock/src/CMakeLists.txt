if(LOCAL_CMAKE_PROJECT_NAME)
	message(STATUS "Integrated on general project with name ${LOCAL_CMAKE_PROJECT_NAME}")
else()
        cmake_minimum_required(VERSION 3.0)
	project(JsonMock NONE)
	set(LOCAL_CMAKE_PROJECT_NAME "${CMAKE_PROJECT_NAME}")
	message(STATUS "Integrated on general project with name ${LOCAL_CMAKE_PROJECT_NAME}")
endif()
set(TEST_TARGET "${LOCAL_CMAKE_PROJECT_NAME}_json_test")

if(TEST_INSTALL_DIR)
	message(STATUS "Using global test installation directory: ${TEST_INSTALL_DIR}")
else()
	set(TEST_INSTALL_DIR "${CMAKE_CURRENT_BINARY_DIR}")
	message(STATUS "Using temporary test installation directory: ${TEST_INSTALL_DIR}")
endif()

find_program(LOCAL_GO_COMPILER go HINTS /home /usr)
message(STATUS "Go compiler: ${LOCAL_GO_COMPILER}")

if(EXISTS ${LOCAL_GO_COMPILER})

	if(WIN32)
		message(STATUS "On Windows systems, try to avoid using paths with blanks in order to place your configuration/data files just in case")
	else()
		message(STATUS "On Unix-like systems, take into account uper and lower case name differencies in your configuration/data files just in case")
	endif()

	# Check out libraries needed
	add_custom_target(${TEST_TARGET}_libs
		COMMAND ${LOCAL_GO_COMPILER} get "github.com/gorilla/mux"
		COMMAND ${LOCAL_GO_COMPILER} get "github.com/xeipuuv/gojsonschema"
	)

	# main mock
	add_custom_target(${TEST_TARGET} ALL ${LOCAL_GO_COMPILER} build ${CMAKE_CURRENT_SOURCE_DIR}/JsonMock.go 
		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_SOURCE_DIR}/../data ${CMAKE_CURRENT_BINARY_DIR}/data 
		DEPENDS ${TEST_TARGET}_libs)

 ### Only if this the principal project ###
 if("${LOCAL_CMAKE_PROJECT_NAME}" STREQUAL "${CMAKE_PROJECT_NAME}")
	add_custom_target(install${TEST_TARGET} ALL ${CMAKE_COMMAND} -E copy_if_different ${CMAKE_CURRENT_BINARY_DIR}/JsonMock${CMAKE_EXECUTABLE_SUFFIX} ${TEST_INSTALL_DIR} 
		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_BINARY_DIR}/data ${TEST_INSTALL_DIR}/data 
		DEPENDS ${TEST_TARGET})
 endif()

 ### Testing ###
 if(${LOCAL_CMAKE_PROJECT_NAME}_TEST)
	add_custom_target(${TEST_TARGET}.test ALL ${LOCAL_GO_COMPILER} test -c ${CMAKE_CURRENT_SOURCE_DIR}/JsonMock_test.go -o JsonMock.test 
		COMMAND ${CMAKE_COMMAND} -E rename ${CMAKE_CURRENT_BINARY_DIR}/JsonMock.test ${CMAKE_CURRENT_BINARY_DIR}/JsonMock_test${CMAKE_EXECUTABLE_SUFFIX}
		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_SOURCE_DIR}/../data ${CMAKE_CURRENT_BINARY_DIR}/data 
		DEPENDS ${TEST_TARGET})

	add_custom_target(${TEST_TARGET}_pattern.test ALL ${LOCAL_GO_COMPILER} test -c ${CMAKE_CURRENT_SOURCE_DIR}/JsonMock_pattern_test.go -o JsonMock_pattern.test
		COMMAND ${CMAKE_COMMAND} -E rename ${CMAKE_CURRENT_BINARY_DIR}/JsonMock_pattern.test ${CMAKE_CURRENT_BINARY_DIR}/JsonMock_pattern_test${CMAKE_EXECUTABLE_SUFFIX}
		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_SOURCE_DIR}/../data ${CMAKE_CURRENT_BINARY_DIR}/data 
		DEPENDS ${TEST_TARGET})

   ### Only if this the principal project ###
   if("${LOCAL_CMAKE_PROJECT_NAME}" STREQUAL "${CMAKE_PROJECT_NAME}")
	   add_custom_target(install${TEST_TARGET}.test ${CMAKE_COMMAND} -E copy_if_different ${CMAKE_CURRENT_BINARY_DIR}/JsonMock_test${CMAKE_EXECUTABLE_SUFFIX} ${TEST_INSTALL_DIR}/JsonMock_test${CMAKE_EXECUTABLE_SUFFIX}
	   		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_BINARY_DIR}/data ${TEST_INSTALL_DIR}/data 
	   		DEPENDS install${TEST_TARGET} ${TEST_TARGET}.test)
	   add_custom_target(test${TEST_TARGET} ./JsonMock_test${CMAKE_EXECUTABLE_SUFFIX} 
	   		DEPENDS install${TEST_TARGET}.test 
	   		WORKING_DIRECTORY ${TEST_INSTALL_DIR})
	   
	    add_custom_target(install${TEST_TARGET}_pattern.test ${CMAKE_COMMAND} -E copy_if_different ${CMAKE_CURRENT_BINARY_DIR}/JsonMock_pattern_test${CMAKE_EXECUTABLE_SUFFIX} ${TEST_INSTALL_DIR}/JsonMock_pattern_test${CMAKE_EXECUTABLE_SUFFIX}
	   		COMMAND ${CMAKE_COMMAND} -E copy_directory ${CMAKE_CURRENT_BINARY_DIR}/data ${TEST_INSTALL_DIR}/data 
	   		DEPENDS install${TEST_TARGET} ${TEST_TARGET}_pattern.test)
	   add_custom_target(test${TEST_TARGET}_pattern ./JsonMock_pattern_test${CMAKE_EXECUTABLE_SUFFIX} 
	   		DEPENDS install${TEST_TARGET}_pattern.test 
	   		WORKING_DIRECTORY ${TEST_INSTALL_DIR})
   endif()

 endif()

else(EXISTS ${LOCAL_GO_COMPILER})

  add_custom_target(${TEST_TARGET} ALL echo "No golang compiler means no ${LOCAL_CMAKE_PROJECT_NAME} go tests")

  ### Testing ###
  if(${LOCAL_CMAKE_PROJECT_NAME}_TEST)
      add_custom_target(${TEST_TARGET}.test ALL echo "No golang compiler means no ${LOCAL_CMAKE_PROJECT_NAME}.test" DEPENDS ${TEST_TARGET})
  endif()

endif(EXISTS ${LOCAL_GO_COMPILER})
