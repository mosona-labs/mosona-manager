package auth

import (
	"context"
	"fmt"
	"mosona-manager/internal/redis"
	"mosona-manager/internal/utils"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	loginFailureWindow = 15 * time.Minute
	loginMaxFailures   = 5
	ipLoginMaxFailures = 30

	totpFailureWindow = 10 * time.Minute
	totpMaxFailures   = 5
)

func checkLoginRateLimit(ctx context.Context, email, ip string) (time.Duration, error) {
	emailRetry, err := retryAfter(ctx, loginEmailIPKey(email, ip), loginMaxFailures)
	if err != nil {
		return 0, err
	}
	ipRetry, err := retryAfter(ctx, loginIPKey(ip), ipLoginMaxFailures)
	if err != nil {
		return 0, err
	}
	if ipRetry > emailRetry {
		return ipRetry, nil
	}
	return emailRetry, nil
}

func recordLoginFailure(ctx context.Context, email, ip string) error {
	if err := incrementFailure(ctx, loginEmailIPKey(email, ip), loginFailureWindow); err != nil {
		return err
	}
	return incrementFailure(ctx, loginIPKey(ip), loginFailureWindow)
}

func clearLoginFailures(ctx context.Context, email, ip string) {
	_ = redis.Client.Del(ctx, loginEmailIPKey(email, ip)).Err()
}

func checkTOTPRateLimit(ctx context.Context, uid int64, ip string) (time.Duration, error) {
	return retryAfter(ctx, totpUIDIPKey(uid, ip), totpMaxFailures)
}

func recordTOTPFailure(ctx context.Context, uid int64, ip string) error {
	return incrementFailure(ctx, totpUIDIPKey(uid, ip), totpFailureWindow)
}

func clearTOTPFailures(ctx context.Context, uid int64, ip string) {
	_ = redis.Client.Del(ctx, totpUIDIPKey(uid, ip)).Err()
}

func retryAfter(ctx context.Context, key string, maxFailures int64) (time.Duration, error) {
	count, err := redis.Client.Get(ctx, key).Int64()
	if err != nil {
		if err == goredis.Nil {
			return 0, nil
		}
		return 0, err
	}
	if count < maxFailures {
		return 0, nil
	}

	ttl, err := redis.Client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if ttl <= 0 {
		return time.Second, nil
	}
	return ttl, nil
}

func incrementFailure(ctx context.Context, key string, window time.Duration) error {
	count, err := redis.Client.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		return redis.Client.Expire(ctx, key, window).Err()
	}
	return nil
}

func loginEmailIPKey(email, ip string) string {
	return "auth:fail:login:email_ip:" + utils.SHA256(email+"|"+ip)
}

func loginIPKey(ip string) string {
	return "auth:fail:login:ip:" + utils.SHA256(ip)
}

func totpUIDIPKey(uid int64, ip string) string {
	return "auth:fail:totp:uid_ip:" + utils.SHA256(fmt.Sprintf("%d|%s", uid, ip))
}
