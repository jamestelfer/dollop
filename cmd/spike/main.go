// Command spike is a THROWAWAY prototype for Phase 1 of the `update` plan
// (docs/update-command.plan.md). It empirically answers one question:
//
//	Does a CopyObject self-copy (same source and destination key) on Cloudflare
//	R2 advance the object's Last-Modified timestamp — and does R2 even permit a
//	self-copy with MetadataDirective: COPY?
//
// This matters because the `update` touch pass (PRD §4.3/§4.4) relies on a
// self-copy to reset the lifecycle expiry clock across a prefix. Standard AWS
// S3 *rejects* a no-op self-copy ("this copy request is illegal because it is
// trying to copy an object to itself without changing the object's metadata,
// storage class, website redirect location or encryption attributes"). R2's
// behaviour for this case is undocumented, hence this spike.
//
// It reuses the same config/keyring wiring as cmd/dollop so it runs against the
// operator's real R2 bucket with no extra setup:
//
//	go run ./cmd/spike            # uses configured bucket, key "spike/touch-probe"
//	go run ./cmd/spike -sleep 5s  # longer gap between T0 and T1
//	go run ./cmd/spike -keep      # leave the scratch object behind for inspection
//
// DELETE THIS PACKAGE once the result is recorded in docs/prd.md (spike gate).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jamestelfer/dollop/internal/config"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "spike failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	key := flag.String("key", "spike/touch-probe", "scratch object key to write under")
	sleep := flag.Duration("sleep", 3*time.Second, "delay between the initial put and the self-copy")
	keep := flag.Bool("keep", false, "leave the scratch object in the bucket instead of deleting it")
	flag.Parse()

	bucket, client, err := newClient()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "bucket=%s key=%s sleep=%s\n", bucket, *key, *sleep)

	// 1. Put a small fixed object under the scratch key.
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(*key),
		ContentType: aws.String("text/plain"),
		Body:        bytes.NewReader([]byte("dollop spike: r2 self-copy probe\n")),
	}); err != nil {
		return fmt.Errorf("put scratch object: %w", err)
	}
	if !*keep {
		defer func() {
			if _, derr := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket), Key: aws.String(*key),
			}); derr != nil {
				fmt.Fprintln(os.Stderr, "cleanup: failed to delete scratch object:", derr)
			}
		}()
	}

	// 2. Capture Last-Modified at T0.
	t0, err := lastModified(ctx, client, bucket, *key)
	if err != nil {
		return fmt.Errorf("head before copy: %w", err)
	}
	fmt.Fprintf(os.Stderr, "T0 Last-Modified: %s\n", t0.Format(time.RFC3339Nano))

	// 3. Wait long enough for a measurable delta.
	time.Sleep(*sleep)

	// 4. Probe each metadata directive in turn, reporting which R2 accepts and
	//    whether each advances Last-Modified. COPY is the directive the PRD
	//    chose; REPLACE and MERGE are the contingencies if COPY is rejected.
	directives := []types.MetadataDirective{
		types.MetadataDirectiveCopy,
		types.MetadataDirectiveReplace,
		// MERGE is an R2 extension; the SDK enum has no constant for it.
		types.MetadataDirective("MERGE"),
	}

	prev := t0
	anyAdvanced := false
	for _, dir := range directives {
		t, advanced, err := selfCopyAndHead(ctx, client, bucket, *key, dir, prev)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  directive %-7s REJECTED: %s\n", dir, summariseErr(err))
			continue
		}
		anyAdvanced = anyAdvanced || advanced
		fmt.Fprintf(os.Stderr, "  directive %-7s accepted; Last-Modified=%s advanced=%t\n",
			dir, t.Format(time.RFC3339Nano), advanced)
		prev = t
	}

	// 5. Verdict.
	fmt.Println("=== SPIKE RESULT ===")
	if anyAdvanced {
		fmt.Println("PASS: at least one self-copy directive advanced Last-Modified on R2.")
		fmt.Println("Record the accepted directive(s) above in docs/prd.md (spike gate).")
	} else {
		fmt.Println("FAIL: no self-copy directive advanced Last-Modified.")
		fmt.Println("The touch approach does not reset the expiry clock as designed —")
		fmt.Println("revise the PRD reconciliation before starting Phase 2.")
	}
	return nil
}

// selfCopyAndHead issues a self-copy with the given directive, then heads the
// object and reports whether Last-Modified advanced past prev. When the
// directive is REPLACE the content-type is re-sent, since REPLACE drops
// unspecified system metadata.
func selfCopyAndHead(
	ctx context.Context, client *s3.Client, bucket, key string,
	dir types.MetadataDirective, prev time.Time,
) (time.Time, bool, error) {
	input := &s3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		CopySource:        aws.String(bucket + "/" + key),
		MetadataDirective: dir,
	}
	if dir == types.MetadataDirectiveReplace {
		input.ContentType = aws.String("text/plain")
	}
	if _, err := client.CopyObject(ctx, input); err != nil {
		return time.Time{}, false, err
	}
	t, err := lastModified(ctx, client, bucket, key)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, t.After(prev), nil
}

func lastModified(ctx context.Context, client *s3.Client, bucket, key string) (time.Time, error) {
	out, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return time.Time{}, err
	}
	if out.LastModified == nil {
		return time.Time{}, fmt.Errorf("HeadObject returned no Last-Modified")
	}
	return *out.LastModified, nil
}

// newClient loads config and credentials exactly as cmd/dollop does, then
// builds an R2-pointed S3 client inline.
func newClient() (string, *s3.Client, error) {
	cfgPath, err := config.ConfigFilePath()
	if err != nil {
		return "", nil, err
	}
	authPath, err := config.AuthFilePath()
	if err != nil {
		return "", nil, err
	}
	readKr := config.NewFallbackStore(config.NewSystemKeyring(), config.NewPlaintextStore(authPath))

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return "", nil, fmt.Errorf("load config: %w", err)
	}
	bucket, err := cfg.Require("bucket")
	if err != nil {
		return "", nil, err
	}
	if cfg.AccountID == "" {
		return "", nil, fmt.Errorf("required config key %q is not set", "account-id")
	}

	accessKey, _, err := readKr.GetWithSource(config.ServiceName, "r2-key")
	if err != nil {
		return "", nil, fmt.Errorf("read r2-key: %w", err)
	}
	secretKey, _, err := readKr.GetWithSource(config.ServiceName, "r2-secret")
	if err != nil {
		return "", nil, fmt.Errorf("read r2-secret: %w", err)
	}
	if accessKey == "" || secretKey == "" {
		return "", nil, fmt.Errorf("R2 credentials are not configured (run `dollop config auth`)")
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
	})
	return bucket, client, nil
}

// summariseErr trims an AWS error to its first line for terse logging.
func summariseErr(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
