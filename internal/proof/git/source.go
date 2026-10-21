package git

import (
	"context"
	"errors"
	"net/netip"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceid"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceurl"
)

type scriptedResolver struct {
	answers [][]netip.Addr
	calls   int
}

func (r *scriptedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	if r.calls >= len(r.answers) {
		return nil, errors.New("unexpected resolver call")
	}
	answer := append([]netip.Addr(nil), r.answers[r.calls]...)
	r.calls++
	return answer, nil
}

const ReviewedPublicSource = sourceid.KnownVectorURI

func validateReviewedSource(raw string) (string, error) {
	if raw == "" {
		raw = ReviewedPublicSource
	}
	if raw != ReviewedPublicSource {
		return "", errors.New("source is not the reviewed canonical public proof identity")
	}
	if err := validateCanonicalHTTPS(raw); err != nil {
		return "", err
	}
	return sourceid.Digest(raw)
}

func sourceIDKnownVectorScenario(actual string) Scenario {
	ids := []string{"PC-022", "VAL-006", "VAL-012"}
	if actual != sourceid.KnownVectorDigest {
		return failed("source-id-shared-known-vector", ids, "the reviewed canonical URI did not encode to the fixed versioned map-free source_id shared by every proof slice")
	}
	return observed("source-id-shared-known-vector", ids, "the reviewed canonical URI encoded to the fixed versioned map-free source_id shared by every proof slice")
}

func validateCanonicalHTTPS(raw string) error {
	return sourceurl.ValidatePublicHTTPS(raw)
}

func pinnedReviewedSource(ctx context.Context, raw string, resolver sourceurl.Resolver) ([]string, []netip.Addr, error) {
	if _, err := validateReviewedSource(raw); err != nil {
		return nil, nil, err
	}
	return sourceurl.PinnedPublicHTTPS(ctx, raw, resolver)
}

func endpointAdmissionFixtures(ctx context.Context) bool {
	privateClasses := []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("100.64.0.1"),
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("169.254.1.1"),
		netip.IPv4Unspecified(),
		netip.MustParseAddr("224.0.0.1"),
		netip.IPv6Loopback(),
		netip.MustParseAddr("fc00::1"),
		netip.MustParseAddr("fe80::1"),
		netip.IPv6Unspecified(),
		netip.MustParseAddr("ff02::1"),
	}
	for _, answer := range privateClasses {
		resolver := &scriptedResolver{answers: [][]netip.Addr{{answer}}}
		if _, _, err := pinnedReviewedSource(ctx, ReviewedPublicSource, resolver); err == nil || resolver.calls != 1 {
			return false
		}
	}
	mixed := &scriptedResolver{answers: [][]netip.Addr{{netip.MustParseAddr("11.0.0.11"), netip.MustParseAddr("10.0.0.11")}}}
	if _, _, err := pinnedReviewedSource(ctx, ReviewedPublicSource, mixed); err == nil || mixed.calls != 1 {
		return false
	}
	first := netip.MustParseAddr("11.0.0.10")
	changed := netip.MustParseAddr("10.0.0.10")
	resolver := &scriptedResolver{answers: [][]netip.Addr{{first}, {changed}}}
	entries, answers, err := pinnedReviewedSource(ctx, ReviewedPublicSource, resolver)
	if err != nil || resolver.calls != 1 || len(entries) != 1 || len(answers) != 1 || answers[0] != first ||
		entries[0] != "github.com:443:"+first.String() {
		return false
	}
	_, _, changedErr := pinnedReviewedSource(ctx, ReviewedPublicSource, resolver)
	return changedErr != nil && resolver.calls == 2
}

func validateStaticGitlink(raw string) (string, error) {
	return sourceurl.ValidateUnselectedGitlink(raw)
}
