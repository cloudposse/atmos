import React from "react";
import clsx from "clsx";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import styles from "./index.module.css";
import ImageGallery from "react-image-gallery";
import "react-image-gallery/styles/css/image-gallery.css"

function Header() {
    const {siteConfig} = useDocusaurusContext();
    return (
        <header className={clsx("hero", styles.heroBanner)}>
            <div className="container">
                <h1 className="hero__title">{siteConfig.title}</h1>
                <p className="hero__subtitle">{siteConfig.tagline}</p>
            </div>
        </header>
    );
}

// https://github.com/xiaolin/react-image-gallery
// https://stackoverflow.com/questions/3746725/how-to-create-an-array-containing-1-n
const images = Array.from({length: 27}, (_, i) => {
        let ix = i + 1;
        return {
            original: "/img/slides/atmos-intro-" + ix + ".svg",
            thumbnail: "/img/slides/atmos-intro-" + ix + ".svg",
            originalAlt: "Atmos introduction slide " + ix,
            originalTitle: "Atmos introduction slide " + ix,
            thumbnailAlt: "Atmos introduction slide " + ix,
            thumbnailTitle: "Atmos introduction slide " + ix,
            loading: "lazy"
        }
    }
);

export default function Index() {
    const {siteConfig} = useDocusaurusContext();
    return (
        <Layout
            title={`${siteConfig.title}`}
            description="Universal tool for DevOps and Cloud Automation">
            <Header/>
            <main>
                <section className={styles.features}>
                    <div className="container">
                        <ImageGallery items={images}
                                      thumbnailPosition={"left"}
                                      showBullets={false}
                                      showIndex={true}
                                      slideInterval={4000}
                                      lazyLoad={true}
                        />
                    </div>
                </section>
            </main>
        </Layout>
    );
}
