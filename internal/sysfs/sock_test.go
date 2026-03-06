package sysfs

import (
	"net"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestTcpConnFile_Write(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	file := newTcpConn(tcp)
	errno := sys.Errno(0)
	// Ensure we don't interrupt until we get a non-zero errno,
	// and we retry on EAGAIN (i.e. when nonblocking is true).
	for {
		_, errno = file.Write([]byte("wazero"))
		if errno != sys.EAGAIN {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Zero(t, errno)

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	bytes := make([]byte, 4)

	n, err := conn.Read(bytes)
	require.NoError(t, err)
	require.NotEqual(t, 0, n)

	require.Equal(t, "waze", string(bytes))
}

func TestTcpConnFile_Read(t *testing.T) {
	// Test #1: Read from a TCP connection with default synchrony
	// (i.e., without explicitly setting the non-blocking flag).
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	n, err := tcp.Write([]byte("wazero"))
	require.NoError(t, err)
	require.NotEqual(t, 0, n)

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	bytes := make([]byte, 4)

	require.NoError(t, err)
	errno := sys.Errno(0)
	file := newTcpConn(conn.(*net.TCPConn))
	// Ensure we don't interrupt until we get a non-zero errno,
	// and we retry on EAGAIN (i.e. when nonblocking is true).
	for {
		_, errno = file.Read(bytes)
		if errno != sys.EAGAIN {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Zero(t, errno)
	require.NoError(t, err)
	require.Equal(t, "waze", string(bytes))

	// Test #2: Read from a TCP connection asynchronously (i.e., with
	// the non-blocking flag set explicitly).
	tcpAddr2, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp2, err := net.DialTCP("tcp", nil, tcpAddr2)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	// Use a goroutine to asynchronously write to the TCP connection
	// with a delay that is visible to the test.
	go func() {
		time.Sleep(200 * time.Millisecond)
		n2, err := tcp2.Write([]byte("wazero"))
		require.NoError(t, err)
		require.NotEqual(t, 0, n2)
	}()

	conn2, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	bytes2 := make([]byte, 4)

	require.NoError(t, err)
	errno2 := sys.Errno(0)
	file2 := newTcpConn(conn2.(*net.TCPConn))
	errno2 = file2.(*tcpConnFile).SetNonblock(true)
	require.Zero(t, errno2)

	// Ensure we start by getting EAGAIN.
	_, errno2 = file2.Read(bytes2)
	require.Equal(t, sys.EAGAIN, errno2)

	// Ensure we don't interrupt until we get a non-zero errno,
	// and we retry on EAGAIN (i.e. when nonblocking is true).
	for {
		_, errno2 = file2.Read(bytes2)
		if errno2 != sys.EAGAIN {
			break
		}
	}
	require.Zero(t, errno2)
	require.NoError(t, err)
	require.Equal(t, "waze", string(bytes2))
}

func TestTcpConnFile_CachedRawConn(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	// Verify that newTcpConn caches rawConn and fd.
	file := newTcpConn(tcp)
	tcf := file.(*tcpConnFile)
	require.NotNil(t, tcf.rawConn, "rawConn should be cached at construction")
	require.NotEqual(t, uintptr(0), tcf.cachedFd, "cachedFd should be non-zero")

	// Verify that the cached rawConn is usable for I/O by doing a
	// nonblocking write (which uses the rawConn.Control fast path).
	errno := file.(*tcpConnFile).SetNonblock(true)
	require.Zero(t, errno)

	_, errno = file.Write([]byte("hello"))
	// EAGAIN is acceptable for nonblocking, but not a hard error.
	if errno != 0 {
		require.Equal(t, sys.EAGAIN, errno)
	}
}

func TestTcpListenerFile_CachedRawConn(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	// Verify that newTCPListenerFile caches rawConn and fd.
	lf := newTCPListenerFile(listen.(*net.TCPListener))
	tlf := lf.(*tcpListenerFile)
	require.NotNil(t, tlf.rawConn, "rawConn should be cached at construction")
	require.NotEqual(t, uintptr(0), tlf.cachedFd, "cachedFd should be non-zero")

	// Verify Fd() returns the cached value.
	require.Equal(t, tlf.cachedFd, tlf.Fd())

	// Verify the cached rawConn is usable by setting nonblock
	// (which uses the rawConn.Control fast path).
	errno := tlf.SetNonblock(true)
	require.Zero(t, errno)
	require.True(t, tlf.IsNonblock())
}

func TestTcpConnFile_Stat(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	file := newTcpConn(tcp)
	_, errno := file.Stat()
	require.Zero(t, errno, "Stat should not fail")
}

func TestTcpConnFile_SetNonblock(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	lf := newTCPListenerFile(listen.(*net.TCPListener))

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	nblf := fsapi.Adapt(lf)
	errno := nblf.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, nblf.IsNonblock())

	conn, errno := lf.Accept()
	require.EqualErrno(t, 0, errno)
	defer conn.Close()

	file := fsapi.Adapt(newTcpConn(tcp))
	errno = file.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, file.IsNonblock())
}
