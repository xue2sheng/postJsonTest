#include <fstream>
#include <atomic>
#include <thread>
#include <iostream>
#include <mutex>

#define BOOST_ERROR_CODE_HEADER_ONLY
#include <boost/system/error_code.hpp>
#include <boost/asio.hpp>

class Service {
	static const std::map<unsigned int, std::string>
	http_status_table;

public:
	Service(std::shared_ptr<boost::asio::ip::tcp::socket> sock) :
		m_sock(sock),
		m_request(4096),
		m_response_status_code(200) // Assume success.
	{};

	void start_handling() {

		boost::asio::async_read_until(*m_sock.get(),
			m_request,
			"\r\n",
			[this](
			const boost::system::error_code& ec,
			std::size_t bytes_transferred)
		{
			on_request_line_received(ec,
				bytes_transferred);
		});

	}

private:
	void on_request_line_received(
		const boost::system::error_code& ec,
		std::size_t bytes_transferred)
	{
		if (ec != 0) {
			std::cout << "Error occured! Error code = "
				<< ec.value()
				<< ". Message: " << ec.message();

			if (ec == boost::asio::error::not_found) {
				// No delimiter has been fonud in the
				// request message.

				m_response_status_code = 413;
				send_response();

				return;
			}
			else {
				// In case of any other error
				// close the socket and clean up.
				on_finish();
				return;
			}
		}

		// Parse the request line.
		std::string request_line;
		std::istream request_stream(&m_request);
		std::getline(request_stream, request_line, '\r');
		// Remove symbol '\n' from the buffer.
		request_stream.get();

		// Parse the request line.
		std::string request_method;
		std::istringstream request_line_stream(request_line);
		request_line_stream >> request_method;

		// Header as a cheap 'ping'
		if (request_method.compare("HEAD") == 0) {
			m_response_status_code = 200;
			send_response();
			return;
		}

		// We only support POST method.
		if (request_method.compare("POST") != 0) {
			// Unsupported method.
			m_response_status_code = 501;
			send_response();

			return;
		}

		std::string ignore_parameters;
		request_line_stream >> ignore_parameters;

		std::string request_http_version;
		request_line_stream >> request_http_version;

		if (request_http_version.compare("HTTP/1.1") != 0) {
			// Unsupported HTTP version or bad request.
			m_response_status_code = 505;
			send_response();

			return;
		}

		// Read Content-length to read all body info later on
		unsigned long content_length{0};
		do {
			 std::getline(request_stream, request_line, '\r');
			 if( request_line.find("Content-Length: ") != std::string::npos ) {
				content_length = std::stoul(request_line.substr(std::string{"Content-Length: "}.size()));
				break;
			 }
		} while( !request_stream.eof() );
		if( content_length == 0 ) {
			// response with empty body if not content length present
			process_request();
			send_response();
		}


		// At this point the request line is successfully
		// received and parsed. Now read the request headers and body.
		boost::asio::async_read_until(*m_sock.get(),
			m_request,
			"\r\n\r\n",
			[this, content_length](
			const boost::system::error_code& ec,
			std::size_t bytes_transferred)
		{
			on_request_received(ec, bytes_transferred, content_length);
		});

		return;
	}

	void on_request_received(const boost::system::error_code& ec, std::size_t bytes_transferred, unsigned long content_length)
	{
		if (ec != 0) {
			std::cout << "Error occured! Error code = "
				<< ec.value()
				<< ". Message: " << ec.message();

			if (ec == boost::asio::error::not_found) {
				// No delimiter has been fonud in the
				// request message.

				m_response_status_code = 413;
				send_response();
				return;
			}
			else {
				// In case of any other error - close the
				// socket and clean up.
				on_finish();
				return;
			}
		}

		// Parse headers and body.
		std::istream request_stream(&m_request);
		std::string whole_line, content_body;


			bool start_body {false}; // hopefully json format
			bool json_content {false}; // activate parsing for json
			do {
				request_stream >> whole_line;

				// awkish triggers to detect json data
				if( not json_content && whole_line.find("application/json") != std::string::npos ) { json_content = true; }
				if( not start_body && json_content && whole_line.find("{") != std::string::npos ) { start_body = true; }

				if( start_body ) { content_body += whole_line; }

			} while (!request_stream.eof());


		// maybe the request body was greater than bytes_transferred and it's missing a part of that content body
		if( content_body.size() < content_length ) {

			std::unique_ptr<char[]> buf;
			unsigned long remaining_length = (content_length - content_body.size());
			buf.reset(new char[remaining_length]);

			unsigned long total_bytes_read{0};
			while( total_bytes_read != remaining_length) {
				total_bytes_read += m_sock.get()->read_some(boost::asio::buffer(buf.get() + total_bytes_read, remaining_length - total_bytes_read));
			}

			std::string extra(buf.get(), buf.get() + strlen(buf.get()));
			content_body += extra;
		}

		// Now we have all we need to process the request.
		process_request(content_body);
		send_response(content_body);

		return;
	}

	void process_request(const std::string& body = "") {

		if (body.empty()) {
			// Resource not found.
			m_response_status_code = 404;
			return;
		}

		m_response_headers += "Content-Type: application/json\r\n";
		m_response_headers += std::string("content-length") + ": " + std::to_string(body.size()) + "\r\n";
	}

	void send_response(const std::string& body = "")  {

		m_sock->shutdown( boost::asio::ip::tcp::socket::shutdown_receive);
		auto status_line = http_status_table.at(m_response_status_code);

		m_response_status_line = std::string("HTTP/1.1 ") + status_line + "\r\n";
		m_response_headers += "\r\n";

		std::vector<boost::asio::const_buffer> response_buffers;
		response_buffers.push_back( boost::asio::buffer(m_response_status_line) );

		if (m_response_headers.length() > 0) {
			response_buffers.push_back( boost::asio::buffer(m_response_headers) );
		}

		if (body.size() > 0) {
			response_buffers.push_back( boost::asio::buffer(body.c_str(), body.size()));
		}

		// Initiate asynchronous write operation.
		boost::asio::async_write(*m_sock.get(),
			response_buffers,
			[this](
			const boost::system::error_code& ec,
			std::size_t bytes_transferred)
		{
			on_response_sent(ec, bytes_transferred);
		});
	}

	void on_response_sent(const boost::system::error_code& ec,
		std::size_t bytes_transferred)
	{
		if (ec != 0) {
			std::cout << "Error occured! Error code = "
				<< ec.value()
				<< ". Message: " << ec.message();
		}

		m_sock->shutdown(boost::asio::ip::tcp::socket::shutdown_both);

		on_finish();
	}

	// Here we perform the cleanup.
	void on_finish() {
		delete this;
	}

private:
	std::shared_ptr<boost::asio::ip::tcp::socket> m_sock;
	boost::asio::streambuf m_request;

	std::unique_ptr<char[]> m_resource_buffer;
	unsigned int m_response_status_code;
	std::string m_response_headers;
	std::string m_response_status_line;
};

const std::map<unsigned int, std::string>
Service::http_status_table =
{
	{ 200, "200 OK" },
	{ 404, "404 Not Found" },
	{ 413, "413 Request Entity Too Large" },
	{ 500, "500 Server Error" },
	{ 501, "501 Not Implemented" },
	{ 505, "505 HTTP Version Not Supported" }
};

class Acceptor {
public:
	Acceptor(boost::asio::io_service& ios, unsigned short port_num) :
		m_ios(ios),
		m_acceptor(m_ios,
		boost::asio::ip::tcp::endpoint(
		boost::asio::ip::address_v4::any(),
		port_num)),
		m_isStopped(false)
	{}

	// Start accepting incoming connection requests.
	void Start() {
		m_acceptor.listen();
		InitAccept();
	}

	// Stop accepting incoming connection requests.
	void Stop() {
		m_isStopped.store(true);
	}

private:
	void InitAccept() {
		std::shared_ptr<boost::asio::ip::tcp::socket>
			sock(new boost::asio::ip::tcp::socket(m_ios));

		m_acceptor.async_accept(*sock.get(),
			[this, sock](
			const boost::system::error_code& error)
		{
			onAccept(error, sock);
		});
	}

	void onAccept(const boost::system::error_code& ec,
		std::shared_ptr<boost::asio::ip::tcp::socket> sock)
	{
		if (ec == 0) {
			(new Service(sock))->start_handling();
		}
		else {
			std::cout << "Error occured! Error code = "
				<< ec.value()
				<< ". Message: " << ec.message();
		}

		// Init next async accept operation if
		// acceptor has not been stopped yet.
		if (!m_isStopped.load()) {
			InitAccept();
		}
		else {
			// Stop accepting incoming connections
			// and free allocated resources.
			m_acceptor.close();
		}
	}

private:
	boost::asio::io_service& m_ios;
	boost::asio::ip::tcp::acceptor m_acceptor;
	std::atomic<bool> m_isStopped;
};

class Server {
public:
	Server() {
		m_work.reset(new boost::asio::io_service::work(m_ios));
	}

	// Start the server.
	void Start(unsigned short port_num,
		unsigned int thread_pool_size) {

		assert(thread_pool_size > 0);

		// Create and strat Acceptor.
		acc.reset(new Acceptor(m_ios, port_num));
		acc->Start();

		// Create specified number of threads and
		// add them to the pool.
		for (unsigned int i = 0; i < thread_pool_size; i++) {
			std::unique_ptr<std::thread> th(
				new std::thread([this]()
				{
					m_ios.run();
				}));

			m_thread_pool.push_back(std::move(th));
		}
	}

	// Stop the server.
	void Stop() {
		acc->Stop();
		m_ios.stop();

		for (auto& th : m_thread_pool) {
			th->join();
		}
	}

private:
	boost::asio::io_service m_ios;
	std::unique_ptr<boost::asio::io_service::work> m_work;
	std::unique_ptr<Acceptor> acc;
	std::vector<std::unique_ptr<std::thread>> m_thread_pool;
};
