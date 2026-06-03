/*
 * config.go - Actions for creating a new config file, which includes new
 * hashing costs and the config file's location.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package actions

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	"github.com/google/fscrypt/crypto"
	"github.com/google/fscrypt/filesystem"
	"github.com/google/fscrypt/metadata"
	"github.com/google/fscrypt/util"
)

// ConfigFileLocation is the location of fscrypt's global settings. This can be
// overridden by the user of this package.
var ConfigFileLocation = "/etc/fscrypt.conf"

// ErrBadConfig is an internal error that indicates that the config struct is invalid.
type ErrBadConfig struct {
	Config          *metadata.Config
	UnderlyingError error
}

func (err *ErrBadConfig) Error() string {
	return fmt.Sprintf(`internal error: config is invalid: %s

	The invalid config is %s`, err.UnderlyingError, err.Config)
}

// ErrBadConfigFile indicates that the config file is invalid.
type ErrBadConfigFile struct {
	Path            string
	UnderlyingError error
}

func (err *ErrBadConfigFile) Error() string {
	return fmt.Sprintf("%q is invalid: %s", err.Path, err.UnderlyingError)
}

// ErrConfigFileExists indicates that the config file already exists.
type ErrConfigFileExists struct {
	Path string
}

func (err *ErrConfigFileExists) Error() string {
	return fmt.Sprintf("%q already exists", err.Path)
}

// ErrNoConfigFile indicates that the config file doesn't exist.
type ErrNoConfigFile struct {
	Path string
}

func (err *ErrNoConfigFile) Error() string {
	return fmt.Sprintf("%q doesn't exist", err.Path)
}

const (
	// Permissions of the config file (global readable)
	configPermissions = 0644
	// Config file should be created for writing and not already exist
	createFlags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
	// 128 MiB is a large enough amount of memory to make the password hash
	// very difficult to brute force on specialized hardware, but small
	// enough to work on most GNU/Linux systems.
	maxMemoryBytes = 128 * 1024 * 1024
)

var (
	timingPassphrase = []byte("I am a fake passphrase")
	timingSalt       = bytes.Repeat([]byte{42}, metadata.SaltLen)
)

// CreateConfigFile creates a new config file at the appropriate location with
// the appropriate hashing costs and encryption parameters. The hashing will be
// configured to take as long as the specified time target. In addition, the
// version of encryption policy to use may be overridden from the default of v1.
func CreateConfigFile(target time.Duration, policyVersion int64) error {
	// Create the config file before computing the hashing costs, so we fail
	// immediately if the program has insufficient permissions.
	configFile, err := filesystem.OpenFileOverridingUmask(ConfigFileLocation,
		createFlags, configPermissions)
	switch {
	case os.IsExist(err):
		return &ErrConfigFileExists{ConfigFileLocation}
	case err != nil:
		return err
	}
	defer configFile.Close()

	config := &metadata.Config{
		Source:  metadata.DefaultSource,
		Options: metadata.DefaultOptions,
	}

	if policyVersion != 0 {
		config.Options.PolicyVersion = policyVersion
	}

	if config.HashCosts, err = getHashingCosts(target); err != nil {
		return err
	}

	log.Printf("Creating config at %q with %v\n", ConfigFileLocation, config)
	return metadata.WriteConfig(config, configFile)
}

// getConfig returns the current configuration struct. Any fields not specified
// in the config file use the system defaults. An error is returned if the
// config file hasn't been setup with CreateConfigFile yet or the config
// contains invalid data.
func getConfig() (*metadata.Config, error) {
	configFile, err := os.Open(ConfigFileLocation)
	switch {
	case os.IsNotExist(err):
		return nil, &ErrNoConfigFile{ConfigFileLocation}
	case err != nil:
		return nil, err
	}
	defer configFile.Close()

	log.Printf("Reading config from %q\n", ConfigFileLocation)
	config, err := metadata.ReadConfig(configFile)
	if err != nil {
		return nil, &ErrBadConfigFile{ConfigFileLocation, err}
	}

	// Use system defaults if not specified
	if config.Source == metadata.SourceType_default {
		config.Source = metadata.DefaultSource
		log.Printf("Falling back to source of %q", config.Source.String())
	}
	if config.Options.Padding == 0 {
		config.Options.Padding = metadata.DefaultOptions.Padding
		log.Printf("Falling back to padding of %d", config.Options.Padding)
	}
	if config.Options.Contents == metadata.EncryptionOptions_default {
		config.Options.Contents = metadata.DefaultOptions.Contents
		log.Printf("Falling back to contents mode of %q", config.Options.Contents)
	}
	if config.Options.Filenames == metadata.EncryptionOptions_default {
		config.Options.Filenames = metadata.DefaultOptions.Filenames
		log.Printf("Falling back to filenames mode of %q", config.Options.Filenames)
	}
	if config.Options.PolicyVersion == 0 {
		config.Options.PolicyVersion = metadata.DefaultOptions.PolicyVersion
		log.Printf("Falling back to policy version of %d", config.Options.PolicyVersion)
	}

	if err := config.CheckValidity(); err != nil {
		return nil, &ErrBadConfigFile{ConfigFileLocation, err}
	}

	return config, nil
}

// getHashingCosts returns hashing costs so that hashing a password will take
// approximately the target time. This is done using the total amount of RAM,
// the number of CPUs present, and by running the passphrase hash many times.
func getHashingCosts(target time.Duration) (*metadata.HashingCosts, error) {
	log.Printf("Finding hashing costs that take %v\n", target)

	// Start out with the minimal possible costs that use all the CPUs.
	parallelism := int64(runtime.NumCPU())
	// golang.org/x/crypto/argon2 only supports parallelism up to 255.
	// For compatibility, don't use more than that amount.
	if parallelism > metadata.MaxParallelism {
		parallelism = metadata.MaxParallelism
	}
	costs := &metadata.HashingCosts{
		Time:            1,
		Memory:          8 * parallelism,
		Parallelism:     parallelism,
		TruncationFixed: true,
	}

	// If even the minimal costs are not fast enough, just return the
	// minimal costs and log a warning.
	t, err := timeHashingCosts(costs)
	if err != nil {
		return nil, err
	}
	log.Printf("Min Costs={%v}\t-> %v\n", costs, t)

	if t > target {
		log.Printf("time exceeded the target of %v.\n", target)
		return costs, nil
	}

	// Now we start doubling the costs until we reach the target.
	memoryKiBLimit := memoryBytesLimit() / 1024
	for {
		// Store a copy of the previous costs
		costsPrev := proto.Clone(costs).(*metadata.HashingCosts)
		tPrev := t

		// Double the memory up to the max, then double the time.
		if costs.Memory < memoryKiBLimit {
			costs.Memory = util.MinInt64(2*costs.Memory, memoryKiBLimit)
		} else {
			costs.Time *= 2
		}

		// If our hashing failed, return the last good set of costs.
		if t, err = timeHashingCosts(costs); err != nil {
			log.Printf("Hashing with costs={%v} failed: %v\n", costs, err)
			return costsPrev, nil
		}
		log.Printf("Costs={%v}\t-> %v\n", costs, t)

		// If we have reached the target time, we return a set of costs
		// based on the linear interpolation between the last two times.
		if t >= target {
			f := float64(target-tPrev) / float64(t-tPrev)
			return &metadata.HashingCosts{
				Time:            betweenCosts(costsPrev.Time, costs.Time, f),
				Memory:          betweenCosts(costsPrev.Memory, costs.Memory, f),
				Parallelism:     costs.Parallelism,
				TruncationFixed: costs.TruncationFixed,
			}, nil
		}
	}
}

// memoryBytesLimit returns the maximum amount of memory we will use for
// passphrase hashing. This will never be more than a reasonable maximum (for
// compatibility) or an 8th the available system RAM.
func memoryBytesLimit() int64 {
	// The sysinfo syscall only fails if given a bad address
	var info unix.Sysinfo_t
	err := unix.Sysinfo(&info)
	util.NeverError(err)

	totalRAMBytes := int64(info.Totalram)
	return util.MinInt64(totalRAMBytes/8, maxMemoryBytes)
}

// betweenCosts returns a cost between a and b. Specifically, it returns the
// floor of a + f*(b-a). This way, f=0 returns a and f=1 returns b.
func betweenCosts(a, b int64, f float64) int64 {
	return a + int64(f*float64(b-a))
}

// timeHashingCosts runs the passphrase hash with the specified costs and
// returns the time it takes to hash the passphrase.
func timeHashingCosts(costs *metadata.HashingCosts) (time.Duration, error) {
	passphrase, err := crypto.NewKeyFromReader(bytes.NewReader(timingPassphrase))
	if err != nil {
		return 0, err
	}
	defer passphrase.Wipe()

	// Be sure to measure CPU time, not wall time (time.Now)
	begin := cpuTimeInNanoseconds()
	hash, err := crypto.PassphraseHash(passphrase, timingSalt, costs)
	if err == nil {
		hash.Wipe()
	}
	end := cpuTimeInNanoseconds()

	// This uses a lot of memory, run the garbage collector
	runtime.GC()

	return time.Duration((end - begin) / costs.Parallelism), nil
}

// cpuTimeInNanoseconds returns the nanosecond count based on the process's CPU usage.
// This number has no absolute meaning, only relative meaning to other calls.
func cpuTimeInNanoseconds() int64 {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_PROCESS_CPUTIME_ID, &ts)
	// ClockGettime fails if given a bad address or on a VERY old system.
	util.NeverError(err)
	return unix.TimespecToNsec(ts)
}
