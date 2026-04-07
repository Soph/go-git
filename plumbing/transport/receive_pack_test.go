package transport

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/compat"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/stretchr/testify/suite"
)

type ReceivePackSuite struct {
	suite.Suite
}

func TestReceivePackSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ReceivePackSuite))
}

func (s *ReceivePackSuite) TestReceivePackAdvertiseV0() {
	testAdvertise(s.T(), ReceivePack, "", false)
}

func (s *ReceivePackSuite) TestReceivePackAdvertiseV2() {
	// TODO: support version 2
	testAdvertise(s.T(), UploadPack, "version=2", false)
}

func (s *ReceivePackSuite) TestReceivePackAdvertiseV1() {
	buf := testAdvertise(s.T(), ReceivePack, "version=1", false)
	s.Containsf(buf.String(), "version 1", "advertisement should contain version 1")
}

func (s *ReceivePackSuite) TestNormalizeObjectHashZeroSHA256() {
	zero := plumbing.NewHash(strings.Repeat("0", formatcfg.SHA256.HexSize()))
	tr := compat.NewTranslator(compat.Formats{
		Native: formatcfg.SHA1,
		Compat: formatcfg.SHA256,
	}, compat.NewMemoryMapping())

	h := normalizeObjectHash(tr, zero)
	s.True(h.IsZero())
	s.Equal(zero.String(), h.String())
}
