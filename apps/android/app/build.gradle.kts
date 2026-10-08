plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "ru.vpnx3.app"

    val releaseStoreFile = providers.environmentVariable("VPNX3_ANDROID_KEYSTORE_PATH")
    val releaseStorePassword = providers.environmentVariable("VPNX3_ANDROID_KEYSTORE_PASSWORD")
    val releaseKeyAlias = providers.environmentVariable("VPNX3_ANDROID_KEY_ALIAS")
    val releaseKeyPassword = providers.environmentVariable("VPNX3_ANDROID_KEY_PASSWORD")

    signingConfigs {
        if (releaseStoreFile.isPresent && releaseStorePassword.isPresent &&
            releaseKeyAlias.isPresent && releaseKeyPassword.isPresent) {
            create("release") {
                storeFile = file(releaseStoreFile.get())
                storePassword = releaseStorePassword.get()
                keyAlias = releaseKeyAlias.get()
                keyPassword = releaseKeyPassword.get()
            }
        }
    }
    compileSdk = 37

    defaultConfig {
        applicationId = "ru.vpnx3.app"
        minSdk = 26
        targetSdk = 36
        val releaseVersion = providers.environmentVariable("VPNX3_RELEASE_VERSION")
            .orElse("0.2.0")
            .get()
        val releaseVersionCode = providers.environmentVariable("VPNX3_ANDROID_VERSION_CODE")
            .orElse("1")
            .get()
            .toInt()

        versionCode = releaseVersionCode
        versionName = releaseVersion

        val controlUrl = providers.gradleProperty("VPNX3_CONTROL_URL")
            .orElse(providers.environmentVariable("VPNX3_CLIENT_CONTROL_URL"))
            .orElse("https://127.0.0.1")
            .get()
        val configPublicKey = providers.gradleProperty("VPNX3_CONFIG_PUBLIC_KEY")
            .orElse(providers.environmentVariable("VPNX3_CONFIG_PUBLIC_KEY"))
            .orElse("")
            .get()
        val trustRootPublicKey = providers.gradleProperty("VPNX3_TRUST_ROOT_PUBLIC_KEY")
            .orElse(providers.environmentVariable("VPNX3_TRUST_ROOT_PUBLIC_KEY"))
            .orElse("")
            .get()
        val releasePublicKey = providers.gradleProperty("VPNX3_RELEASE_PUBLIC_KEY")
            .orElse(providers.environmentVariable("VPNX3_RELEASE_PUBLIC_KEY"))
            .orElse("")
            .get()
        val bootstrapConfigUrls = providers.gradleProperty("VPNX3_CONFIG_BOOTSTRAP_URLS")
            .orElse(providers.environmentVariable("VPNX3_CONFIG_BOOTSTRAP_URLS"))
            .orElse("")
            .get()
        val bootstrapList = bootstrapConfigUrls.split(',', ';')
            .map { it.trim() }
            .filter { it.isNotEmpty() }

        buildConfigField("String", "CONTROL_URL", "\"$controlUrl\"")
        buildConfigField("String", "CONFIG_PUBLIC_KEY", "\"$configPublicKey\"")
        buildConfigField("String", "TRUST_ROOT_PUBLIC_KEY", "\"$trustRootPublicKey\"")
        buildConfigField("String", "RELEASE_PUBLIC_KEY", "\"$releasePublicKey\"")
        buildConfigField(
            "String",
            "CONFIG_BOOTSTRAP_URLS",
            "\"" + bootstrapList.joinToString(",") + "\""
        )

        if (gradle.startParameter.taskNames.any { it.contains("Release", ignoreCase = true) }) {
            require(controlUrl.startsWith("https://") && !controlUrl.contains("127.0.0.1")) {
                "Release build requires VPNX3_CLIENT_CONTROL_URL or VPNX3_CONTROL_URL with public HTTPS endpoint"
            }
            require(trustRootPublicKey.isNotBlank() || configPublicKey.isNotBlank()) {
                "Release build requires VPNX3_TRUST_ROOT_PUBLIC_KEY or legacy VPNX3_CONFIG_PUBLIC_KEY"
            }
            require(bootstrapList.all {
                it.startsWith("https://") &&
                    !it.contains("localhost", ignoreCase = true) &&
                    !it.contains("127.0.0.1")
            }) {
                "VPNX3_CONFIG_BOOTSTRAP_URLS must contain only public HTTPS URLs"
            }
        }
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        isCoreLibraryDesugaringEnabled = true
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.09.00")
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.11.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-ktx:2.11.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.11.0")
    implementation("com.google.crypto.tink:tink-android:1.23.0")
    implementation("com.wireguard.android:tunnel:1.0.20260102")
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")

    debugImplementation("androidx.compose.ui:ui-tooling")
}
