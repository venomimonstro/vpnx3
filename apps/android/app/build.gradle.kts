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
        versionCode = 1
        versionName = "0.2.0"

        val controlUrl = providers.gradleProperty("VPNX3_CONTROL_URL")
            .orElse("https://127.0.0.1")
            .get()
        val configPublicKey = providers.gradleProperty("VPNX3_CONFIG_PUBLIC_KEY")
            .orElse("")
            .get()
        val releasePublicKey = providers.gradleProperty("VPNX3_RELEASE_PUBLIC_KEY")
            .orElse("")
            .get()

        buildConfigField("String", "CONTROL_URL", "\"$controlUrl\"")
        buildConfigField("String", "CONFIG_PUBLIC_KEY", "\"$configPublicKey\"")
        buildConfigField("String", "RELEASE_PUBLIC_KEY", "\"$releasePublicKey\"")
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
