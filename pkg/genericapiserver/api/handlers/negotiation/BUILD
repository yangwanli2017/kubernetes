package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["negotiate_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//pkg/apis/meta/v1:go_default_library",
        "//pkg/runtime:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "errors.go",
        "negotiate.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//pkg/apis/meta/v1:go_default_library",
        "//pkg/runtime:go_default_library",
        "//pkg/runtime/schema:go_default_library",
        "//vendor:bitbucket.org/ww/goautoneg",
    ],
)
